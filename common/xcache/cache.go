// Package xcache 提供基于 Redis 的 cache-aside 缓存：
//   - GetOrLoad：查缓存未命中时回源，并用 singleflight 合并并发回源（防缓存击穿）；
//   - 负缓存：loader 返回 ErrMiss 表示"确实不存在"，写入短 TTL 的负标记（防缓存穿透）；
//   - Invalidate：显式失效（写后失效），best-effort；
//   - nil 安全：rdb 为 nil 时所有方法退化为直接回源，便于本地无 Redis 运行。
//
// 失效契约：调用方负责在数据变更后调用 Invalidate。例如 user-rpc 缓存了
// user:validate:<id>，那么任何修改用户（用户名等）的写路径——无论是 user-rpc
// 自身还是 user-api——都必须 Invalidate 该 key。当前仅 user-rpc 用到缓存；
// 将来新增 UpdateProfile/ChangeUsername 时务必接入失效。TTL 是兜底安全网，
// 即便漏掉失效，陈旧也限定在 posTTL/negTTL 窗口内。
package xcache

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/zeromicro/go-zero/core/logx"
	"golang.org/x/sync/singleflight"
)

// ErrMiss 由 loader 返回，表示"数据确实不存在"。GetOrLoad 会据此写入负缓存。
var ErrMiss = errors.New("xcache: cache miss (negative)")

// negativeMarker 是负缓存键的字面值，与正缓存的 JSON 载荷区分。
const negativeMarker = "__negative__"

// Cache 是 cache-aside 缓存。零值/nil rdb 也可安全使用（退化为直接回源）。
type Cache struct {
	rdb    *redis.Client
	sf     singleflight.Group
	prefix string
	posTTL time.Duration // 正缓存 TTL
	negTTL time.Duration // 负缓存 TTL（通常较短）
}

// New 创建缓存。prefix 会拼到每个 key 前。rdb 为 nil 时退化为直接回源。
func New(rdb *redis.Client, prefix string, posTTL, negTTL time.Duration) *Cache {
	return &Cache{
		rdb:    rdb,
		prefix: prefix,
		posTTL: posTTL,
		negTTL: negTTL,
	}
}

func (c *Cache) key(k string) string {
	return c.prefix + k
}

// GetOrLoad 先查缓存；未命中时用 singleflight 合并并发回源，loader 返回 ErrMiss
// 写负缓存、返回其它错误不缓存。marshal/unmarshal 由调用方提供，避免本包依赖业务模型。
//
// 注意 singleflight 的固有取舍：同一 key 的并发请求只会回源一次，使用首个请求的 ctx；
// 若首个请求被取消，其它等待者也会收到该错误（标准 singleflight 限制）。
func GetOrLoad[T any](
	ctx context.Context,
	c *Cache,
	key string,
	marshal func(T) ([]byte, error),
	unmarshal func([]byte) (T, error),
	loader func(ctx context.Context) (T, error),
) (T, error) {
	var zero T
	if c == nil || c.rdb == nil {
		// 无缓存后端：直接回源，不做合并（无缓存层时无所谓击穿）。
		return loader(ctx)
	}

	full := c.key(key)

	// 1. 先查缓存。
	if val, err := c.rdb.Get(ctx, full).Bytes(); err == nil {
		if string(val) == negativeMarker {
			return zero, ErrMiss
		}
		if v, uerr := unmarshal(val); uerr == nil {
			return v, nil
		}
		// 反序列化失败（缓存格式过期等），清理脏 key 后回源。
		c.rdb.Del(ctx, full)
	}
	// redis.Nil 或任何 Get 错误都视为未命中。

	// 2. singleflight 合并并发回源（防击穿）。
	raw, err, _ := c.sf.Do(full, func() (any, error) {
		v, lerr := loader(ctx)
		if lerr != nil {
			if errors.Is(lerr, ErrMiss) {
				// 负缓存。
				if serr := c.rdb.Set(ctx, full, negativeMarker, c.negTTL).Err(); serr != nil {
					logx.Errorf("xcache: set negative cache failed: %v", serr)
				}
				return nil, ErrMiss
			}
			// 瞬时错误：不缓存，直接返回。
			return nil, lerr
		}
		// 正缓存。marshal 失败则不缓存但照常返回值。
		if b, merr := marshal(v); merr == nil {
			if serr := c.rdb.Set(ctx, full, b, c.posTTL).Err(); serr != nil {
				logx.Errorf("xcache: set positive cache failed: %v", serr)
			}
		} else {
			logx.Errorf("xcache: marshal failed: %v", merr)
		}
		return v, nil
	})
	if err != nil {
		return zero, err
	}
	return raw.(T), nil
}

// Invalidate 删除指定 key（best-effort，错误只记日志不返回，写路径不会因缓存失败而失败）。
func (c *Cache) Invalidate(ctx context.Context, keys ...string) {
	if c == nil || c.rdb == nil || len(keys) == 0 {
		return
	}
	full := make([]string, len(keys))
	for i, k := range keys {
		full[i] = c.key(k)
	}
	if err := c.rdb.Del(ctx, full...).Err(); err != nil {
		logx.Errorf("xcache: invalidate failed: %v", err)
	}
}

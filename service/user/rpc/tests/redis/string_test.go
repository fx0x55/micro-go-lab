//go:build redis

package redis_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// String 字符串 — Redis 最基础、最常用的数据类型
// ============================================================================
//
// 【为什么用 String】
// Redis 的 String 是二进制安全的，可以存字符串、整数、浮点数、JSON、甚至二进制数据。
// 它是 Redis 万能积木——很多高级功能（计数器、分布式锁、缓存）都基于 String 实现。
//
// 【适用场景】
//   - 缓存：Session、Token、API 响应、数据库查询结果（最常见）
//   - 计数器：文章阅读量、点赞数、限流计数（INCR/DECR 原子操作）
//   - 分布式锁：SET key value NX EX（setnx 语义）
//   - 位运算：签到、打卡（BITCOUNT、BITOP）
//   - 存储 JSON：用户配置、序列化的对象
//
// 【坑和注意事项】
//   1. 最大 512MB，别把大文件往里塞
//   2. INCR 对非整数字符串会报错，不是自动转换
//   3. SET 的 NX/XX 参数互斥，不要同时用
//   4. 批量操作用 MSET/MGET 而不是循环 SET/GET（减少 RTT）
//   5. 值越大序列化后越大，String 存大 JSON 不如 Hash 省内存（listpack 优化）
//   6. SETEX 是原子的（SET + EXPIRE），不要分两步做 SET 和 EXPIRE（非原子，可能丢失 TTL）

func TestString_BasicOps(t *testing.T) {
	key := testKey("string:basic")

	// SET / GET —— 最基础的键值操作
	err := rdb.Set(ctx, key, "hello redis", 0).Err() // 0 = 永不过期
	require.NoError(t, err)

	val, err := rdb.Get(ctx, key).Result()
	require.NoError(t, err)
	assert.Equal(t, "hello redis", val)

	// DEL 删除
	err = rdb.Del(ctx, key).Err()
	require.NoError(t, err)

	// GET 不存在的 key 返回 redis.Nil
	_, err = rdb.Get(ctx, key).Result()
	assert.ErrorIs(t, err, redis.Nil)
}

func TestString_WithExpiry(t *testing.T) {
	key := testKey("string:ttl")

	// SET with TTL —— 验证过期时间
	err := rdb.Set(ctx, key, "expiring", 10*time.Second).Err()
	require.NoError(t, err)

	ttl, err := rdb.TTL(ctx, key).Result()
	require.NoError(t, err)
	// TTL 应该在 9~10 秒之间
	assert.True(t, ttl > 8*time.Second && ttl <= 10*time.Second)

	// PERSIST 去掉 TTL，变为永久
	err = rdb.Persist(ctx, key).Err()
	require.NoError(t, err)

	ttl, err = rdb.TTL(ctx, key).Result()
	require.NoError(t, err)
	assert.Equal(t, time.Duration(-1), ttl) // -1 = 永不过期

	rdb.Del(ctx, key)
}

func TestString_SetNX_DistributedLock(t *testing.T) {
	lockKey := testKey("string:lock")

	// SET NX EX —— 分布式锁的最简实现
	// NX: 只在 key 不存在时设置（加锁）
	// EX: 过期时间（防止进程崩溃后锁永远不释放）
	ok, err := rdb.SetNX(ctx, lockKey, "holder-1", 10*time.Second).Result()
	require.NoError(t, err)
	assert.True(t, ok, "第一次加锁应该成功")

	// 同一个 key 再次加锁应该失败
	ok, err = rdb.SetNX(ctx, lockKey, "holder-2", 10*time.Second).Result()
	require.NoError(t, err)
	assert.False(t, ok, "已被占用的锁不应该再次获取成功")

	rdb.Del(ctx, lockKey)

	// 【坑】生产环境分布式锁需要：
	//   1. value 用随机 UUID，释放时校验是否是自己加的锁（防止误删别人的锁）
	//   2. 用 Lua 脚本做 check-and-delete（GET + DEL 不是原子操作）
	//   3. 锁续期（看门狗机制），防止业务没处理完锁就过期了
	//   4. 推荐用 Redisson/Redlock 等成熟方案
}

func TestString_IncrDecr_Counter(t *testing.T) {
	key := testKey("string:counter")

	// INCR —— 原子自增，适合计数器场景
	// 底层是单线程执行，不需要额外加锁
	err := rdb.Set(ctx, key, 0, 0).Err()
	require.NoError(t, err)

	rdb.Incr(ctx, key)      // +1
	rdb.IncrBy(ctx, key, 5) // +5
	val, _ := rdb.Get(ctx, key).Result()
	assert.Equal(t, "6", val)

	rdb.Decr(ctx, key)      // -1
	rdb.DecrBy(ctx, key, 2) // -2
	val, _ = rdb.Get(ctx, key).Result()
	assert.Equal(t, "3", val)

	rdb.Del(ctx, key)

	// 【坑】INCR 对不存在的 key 会自动创建（值为 0 然后 +1），这是特性不是 bug
	// 【坑】INCRBYFLOAT 对非数字字符串会报错 ERR value is not a valid float
	// 【坑】INCR 返回的是字符串形式的数字，要用 strconv.Atoi 转换
}

func TestString_MultiKey(t *testing.T) {
	// MSET / MGET —— 批量操作，减少网络 RTT
	// 用 pipeline 也能达到类似效果，但 MSET/MGET 更简洁

	keys := []string{testKey("string:m1"), testKey("string:m2"), testKey("string:m3")}
	values := []string{"val1", "val2", "val3"}

	err := rdb.MSet(ctx, keys[0], values[0], keys[1], values[1], keys[2], values[2]).Err()
	require.NoError(t, err)

	results, err := rdb.MGet(ctx, keys...).Result()
	require.NoError(t, err)
	assert.Equal(t, []any{"val1", "val2", "val3"}, results)

	// MGET 不存在的 key 返回 nil
	results, err = rdb.MGet(ctx, keys[0], testKey("string:nonexist")).Result()
	require.NoError(t, err)
	assert.Equal(t, "val1", results[0])
	assert.Nil(t, results[1]) // 不存在的返回 nil

	rdb.Del(ctx, keys...)
}

func TestString_Pipeline(t *testing.T) {
	// Pipeline —— 减少 RTT 的另一种方式
	// 将多个命令打包发送，Redis 依次执行后一次性返回结果
	// 与 MSET/MGET 的区别：Pipeline 可以混合不同类型的命令

	pipe := rdb.Pipeline()
	pipe.Set(ctx, testKey("string:pipe1"), "a", 0)
	pipe.Set(ctx, testKey("string:pipe2"), "b", 0)
	pipe.Incr(ctx, testKey("string:pipe1")) // 这里会报错，"a" 不是整数
	pipe.Get(ctx, testKey("string:pipe2"))

	cmds, _ := pipe.Exec(ctx)

	// 注意：Pipeline 中某个命令失败不影响其他命令执行
	assert.NoError(t, cmds[0].Err()) // SET 成功
	assert.NoError(t, cmds[1].Err()) // SET 成功
	require.Error(t, cmds[2].Err())  // INCR "a" 失败
	assert.Equal(t, "b", cmds[3].(*redis.StringCmd).Val())

	rdb.Del(ctx, testKey("string:pipe1"), testKey("string:pipe2"))
}

func TestString_Append_GetSet(t *testing.T) {
	key := testKey("string:append")

	rdb.Set(ctx, key, "hello", 0)

	// APPEND —— 字符串追加
	rdb.Append(ctx, key, " world")
	val, _ := rdb.Get(ctx, key).Result()
	assert.Equal(t, "hello world", val)

	// STRLEN —— 字节长度（中文在 UTF-8 下占 3 字节）
	rdb.Set(ctx, testKey("string:len"), "中文", 0)
	length, _ := rdb.StrLen(ctx, testKey("string:len")).Result()
	assert.Equal(t, int64(6), length) // "中文" = 6 bytes

	// SET GET —— 原子地设置新值并返回旧值（Redis 6.2+ 推荐用法，替代已废弃的 GETSET）
	old, _ := rdb.SetArgs(ctx, key, "new value", redis.SetArgs{Get: true}).Result()
	assert.Equal(t, "hello world", old)

	rdb.Del(ctx, key, testKey("string:len"))
}

func TestString_Incr_LuaScript(t *testing.T) {
	key := testKey("string:rate_limit")

	// 用 Lua 脚本实现滑动窗口限流的核心逻辑（这里简化为固定窗口）
	// INCR + EXPIRE 不是原子的——如果 INCR 后进程崩溃，key 永远不会过期
	// Lua 脚本在 Redis 中是原子执行的
	script := redis.NewScript(`
		local current = redis.call('INCR', KEYS[1])
		if current == 1 then
			redis.call('EXPIRE', KEYS[1], ARGV[1])
		end
		return current
	`)

	val, err := script.Run(ctx, rdb, []string{key}, 60).Int64()
	require.NoError(t, err)
	assert.Equal(t, int64(1), val) // 第一次调用，值为 1 并设置 60s TTL

	val, _ = script.Run(ctx, rdb, []string{key}, 60).Int64()
	assert.Equal(t, int64(2), val) // 第二次调用，值为 2

	// 检查 TTL 确保被设置了
	ttl, _ := rdb.TTL(ctx, key).Result()
	assert.Positive(t, ttl)

	rdb.Del(ctx, key)
}

func TestString_BinaryData(t *testing.T) {
	key := testKey("string:binary")

	// String 是二进制安全的，可以存任意字节
	binaryData := []byte{0x00, 0xFF, 0x01, 0xFE}
	err := rdb.Set(ctx, key, binaryData, 0).Err()
	require.NoError(t, err)

	got, err := rdb.Get(ctx, key).Bytes()
	require.NoError(t, err)
	assert.Equal(t, binaryData, got)

	rdb.Del(ctx, key)
}

func TestString_JsonSerialization(t *testing.T) {
	key := testKey("string:json")

	type User struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	user := User{Name: "张三", Age: 28}
	data, _ := json.Marshal(user) // 序列化为 JSON

	err := rdb.Set(ctx, key, data, 5*time.Minute).Err()
	require.NoError(t, err)

	raw, err := rdb.Get(ctx, key).Bytes()
	require.NoError(t, err)

	var got User
	err = json.Unmarshal(raw, &got)
	require.NoError(t, err)
	assert.Equal(t, user, got)

	rdb.Del(ctx, key)

	// 【坑】生产中推荐用 msgpack 或 protobuf 代替 JSON，体积更小、反序列化更快
	// 【坑】大 JSON（>1MB）考虑拆分为 Hash，利用 listpack 内存优化
}

//go:build redis

package redis_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	rdb *redis.Client
	ctx context.Context
)

// TestMain 启动所有测试前连接 Redis，测试结束后清理。
// 连接地址可通过环境变量 REDIS_ADDR 覆盖，默认 localhost:6379。
func TestMain(m *testing.M) {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}

	rdb = redis.NewClient(&redis.Options{
		Addr:        addr,
		Password:    "",
		DB:          15, // 用 DB 15 隔离测试数据，避免污染业务数据
		DialTimeout: 3 * time.Second,
	})

	ctx = context.Background()

	if err := rdb.Ping(ctx).Err(); err != nil {
		panic("redis not available: " + err.Error())
	}

	code := m.Run()

	// 清理所有测试 key
	rdb.FlushDB(ctx)
	rdb.Close()
	os.Exit(code)
}

// testKey 返回带前缀的测试 key，避免冲突
func testKey(parts ...string) string {
	prefix := "redis_demo:"
	var prefixSb49 strings.Builder
	for _, p := range parts {
		prefixSb49.WriteString(p)
	}
	prefix += prefixSb49.String()
	return prefix
}

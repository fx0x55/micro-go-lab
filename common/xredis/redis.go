package xredis

import (
	"context"
	"fmt"
	"time"

	"github.com/fx0x55/micro-go-lab/common/config"
	"github.com/redis/go-redis/v9"
)

// New 创建 Redis 客户端并验证连接。
func New(ctx context.Context, cfg config.RedisConfig) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr(),
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("connect redis: %w", err)
	}
	return client, nil
}

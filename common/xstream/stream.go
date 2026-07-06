package xstream

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/fx0x55/micro-go-lab/common/xmetrics"
	"github.com/redis/go-redis/v9"
	"github.com/zeromicro/go-zero/core/logx"
)

const (
	maxBackoff     = 10 * time.Second
	pendingIdleMin = 30 * time.Second
	pendingCount   = 10
)

// consumerCaller 用于日志标识 panic 发生在哪个消费者 goroutine。
func consumerCaller(cfg ConsumerConfig) string {
	return "consumer:" + cfg.Group + "/" + cfg.Name
}

// ConsumerConfig 消费者配置
type ConsumerConfig struct {
	Group  string
	Stream string
	Name   string // consumer name
}

// Handler 消息处理函数
type Handler func(values map[string]string) error

// Producer Redis Streams 生产者
type Producer struct {
	rdb    *redis.Client
	maxLen int64
}

// NewProducer 创建 Redis Streams 生产者
func NewProducer(rdb *redis.Client) *Producer {
	return &Producer{rdb: rdb, maxLen: 10000}
}

// Publish 写入一条消息到 stream
func (p *Producer) Publish(ctx context.Context, stream string, values map[string]string) (string, error) {
	return p.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		MaxLen: p.maxLen,
		Approx: true,
		Values: values,
	}).Result()
}

// Consumer Redis Streams 消费者
type Consumer struct {
	rdb     *redis.Client
	cfg     ConsumerConfig
	handler Handler
}

// NewConsumer 创建 Redis Streams 消费者
func NewConsumer(rdb *redis.Client, cfg ConsumerConfig, handler Handler) *Consumer {
	return &Consumer{
		rdb:     rdb,
		cfg:     cfg,
		handler: handler,
	}
}

// ensureGroup 创建 consumer group（已存在则忽略）。
func (c *Consumer) ensureGroup(ctx context.Context) {
	if err := c.rdb.XGroupCreateMkStream(
		ctx, c.cfg.Stream, c.cfg.Group, "0",
	).Err(); err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "BUSYGROUP") {
			logx.Info("consumer group already exists",
				logx.Field("stream", c.cfg.Stream),
				logx.Field("group", c.cfg.Group))
		} else {
			logx.Error("failed to create consumer group",
				logx.Field("stream", c.cfg.Stream),
				logx.Field("group", c.cfg.Group),
				logx.Field("error", errMsg))
		}
	}
}

// Start 启动消费者（在 goroutine 中运行）。
// ctx 取消时 goroutine 安全退出；wg 用于等待 goroutine 结束。
func (c *Consumer) Start(ctx context.Context, wg *sync.WaitGroup) {
	wg.Add(1)
	go c.consume(ctx, wg)
}

func (c *Consumer) claimPending(ctx context.Context) {
	cursor := "0-0"
	for {
		if ctx.Err() != nil {
			return
		}
		msgs, newCursor, err := c.rdb.XAutoClaim(ctx, &redis.XAutoClaimArgs{
			Stream:   c.cfg.Stream,
			Group:    c.cfg.Group,
			Consumer: c.cfg.Name,
			MinIdle:  pendingIdleMin,
			Start:    cursor,
			Count:    pendingCount,
		}).Result()
		if err != nil {
			logx.Error("XAUTOCLAIM failed",
				logx.Field("stream", c.cfg.Stream),
				logx.Field("error", err.Error()))
			return
		}

		for _, msg := range msgs {
			if err := c.handler(toStringMap(msg.Values)); err != nil {
				logx.Error("pending message handler failed",
					logx.Field("stream", c.cfg.Stream),
					logx.Field("id", msg.ID),
					logx.Field("error", err.Error()))
				xmetrics.RedisStreamMessagesConsumed.WithLabelValues(c.cfg.Stream, c.cfg.Name, "error").Inc()
				continue
			}
			_ = c.rdb.XAck(ctx, c.cfg.Stream, c.cfg.Group, msg.ID).Err()
			xmetrics.RedisStreamMessagesConsumed.WithLabelValues(c.cfg.Stream, c.cfg.Name, "success").Inc()
		}

		if len(msgs) > 0 {
			logx.Info("claimed pending messages",
				logx.Field("stream", c.cfg.Stream),
				logx.Field("count", len(msgs)))
		}

		if newCursor == "0-0" {
			return
		}
		cursor = newCursor
	}
}

func (c *Consumer) consume(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	caller := consumerCaller(c.cfg)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	c.ensureGroup(ctx)

	length, _ := c.rdb.XLen(ctx, c.cfg.Stream).Result()
	logx.Info("consumer starting",
		logx.Field("stream", c.cfg.Stream),
		logx.Field("group", c.cfg.Group),
		logx.Field("consumer", c.cfg.Name),
		logx.Field("stream_length", length))

	// 启动时先认领上次未 ACK 的 pending 消息（at-least-once 保证）
	c.claimPending(ctx)

	logx.Info("consumer loop started",
		logx.Field("stream", c.cfg.Stream),
		logx.Field("group", c.cfg.Group),
		logx.Field("consumer", c.cfg.Name))

	backoff := time.Second
	for {
		select {
		case <-ctx.Done():
			logx.Info("consumer loop stopping",
				logx.Field("stream", c.cfg.Stream),
				logx.Field("consumer", c.cfg.Name))
			return
		default:
		}

		var readErr error
		RunWithRecover(ctx, caller, func(ctx context.Context) {
			streams, err := c.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
				Group:    c.cfg.Group,
				Consumer: c.cfg.Name,
				Streams:  []string{c.cfg.Stream, ">"},
				Count:    10,
				Block:    2 * time.Second,
			}).Result()
			if err != nil {
				readErr = err
				return
			}

			msgCount := 0
			for _, stream := range streams {
				for _, msg := range stream.Messages {
					msgCount++
					strValues := toStringMap(msg.Values)
					if err := c.handler(strValues); err != nil {
						logx.Error("stream handler failed",
							logx.Field("stream", c.cfg.Stream),
							logx.Field("id", msg.ID),
							logx.Field("error", err.Error()),
						)
						xmetrics.RedisStreamMessagesConsumed.WithLabelValues(c.cfg.Stream, c.cfg.Name, "error").Inc()
						continue
					}
					_ = c.rdb.XAck(ctx, c.cfg.Stream, c.cfg.Group, msg.ID).Err()
					xmetrics.RedisStreamMessagesConsumed.WithLabelValues(c.cfg.Stream, c.cfg.Name, "success").Inc()
				}
			}
			if msgCount > 0 {
				logx.Info("consumer batch processed",
					logx.Field("stream", c.cfg.Stream),
					logx.Field("consumer", c.cfg.Name),
					logx.Field("count", msgCount))
			}
		})

		if readErr != nil {
			if errors.Is(readErr, context.Canceled) {
				return
			}

			// NOGROUP: consumer group 丢失（Redis 重启/key 被删），自动重建。
			// context.Canceled 在上方已拦截，到达此处 ctx 必定未取消。
			if strings.Contains(readErr.Error(), "NOGROUP") {
				logx.Info("consumer group missing, recreating",
					logx.Field("stream", c.cfg.Stream),
					logx.Field("group", c.cfg.Group))
				c.ensureGroup(ctx)
			}

			// 指数退避，避免 Redis 不可用时疯狂重试打满日志
			logx.Error("redis stream read failed, retrying",
				logx.Field("stream", c.cfg.Stream),
				logx.Field("error", readErr.Error()),
				logx.Field("backoff", backoff))

			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff = min(backoff*2, maxBackoff)
		} else {
			backoff = time.Second // 成功读取，重置退避
			// 每轮成功读取后，扫描是否有残留 pending 消息
			c.claimPending(ctx)
		}
	}
}

// toStringMap 将 map[string]interface{} 转换为 map[string]string
func toStringMap(m map[string]any) map[string]string {
	result := make(map[string]string, len(m))
	for k, v := range m {
		result[k] = fmt.Sprintf("%v", v)
	}
	return result
}

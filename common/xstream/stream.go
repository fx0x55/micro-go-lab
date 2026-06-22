package xstream

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/fx0x55/micro-go-lab/common/xmetrics"
	"github.com/redis/go-redis/v9"
	"github.com/zeromicro/go-zero/core/logx"
)

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
func (p *Producer) Publish(stream string, values map[string]string) (string, error) {
	return p.rdb.XAdd(context.Background(), &redis.XAddArgs{
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
	cancel  context.CancelFunc
	done    chan struct{}
}

// NewConsumer 创建 Redis Streams 消费者
func NewConsumer(rdb *redis.Client, cfg ConsumerConfig, handler Handler) *Consumer {
	return &Consumer{
		rdb:     rdb,
		cfg:     cfg,
		handler: handler,
		done:    make(chan struct{}),
	}
}

// Start 启动消费者（在 goroutine 中运行）
func (c *Consumer) Start() {
	// 确保 consumer group 存在
	if err := c.rdb.XGroupCreateMkStream(context.Background(), c.cfg.Stream, c.cfg.Group, "0").Err(); err != nil {
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

	// 诊断：启动时检查 stream 状态
	length, _ := c.rdb.XLen(context.Background(), c.cfg.Stream).Result()
	logx.Info("consumer starting",
		logx.Field("stream", c.cfg.Stream),
		logx.Field("group", c.cfg.Group),
		logx.Field("consumer", c.cfg.Name),
		logx.Field("stream_length", length))

	go c.consume()
}

func (c *Consumer) consume() {
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	defer cancel()

	logx.Info("consumer loop started",
		logx.Field("stream", c.cfg.Stream),
		logx.Field("group", c.cfg.Group),
		logx.Field("consumer", c.cfg.Name))

	for {
		select {
		case <-c.done:
			logx.Info("consumer loop stopping",
				logx.Field("stream", c.cfg.Stream),
				logx.Field("consumer", c.cfg.Name))
			return
		default:
		}

		streams, err := c.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    c.cfg.Group,
			Consumer: c.cfg.Name,
			Streams:  []string{c.cfg.Stream, ">"},
			Count:    10,
			Block:    2 * time.Second,
		}).Result()
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			if errors.Is(err, redis.Nil) {
				continue
			}
			logx.Error("redis stream read failed",
				logx.Field("stream", c.cfg.Stream),
				logx.Field("error", err.Error()))
			continue
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
				_ = c.rdb.XAck(context.Background(), c.cfg.Stream, c.cfg.Group, msg.ID).Err()
				xmetrics.RedisStreamMessagesConsumed.WithLabelValues(c.cfg.Stream, c.cfg.Name, "success").Inc()
			}
		}
		if msgCount > 0 {
			logx.Info("consumer batch processed",
				logx.Field("stream", c.cfg.Stream),
				logx.Field("consumer", c.cfg.Name),
				logx.Field("count", msgCount))
		}
	}
}

// Stop 停止消费者
func (c *Consumer) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
	close(c.done)
}

// toStringMap 将 map[string]interface{} 转换为 map[string]string
func toStringMap(m map[string]any) map[string]string {
	result := make(map[string]string, len(m))
	for k, v := range m {
		result[k] = fmt.Sprintf("%v", v)
	}
	return result
}

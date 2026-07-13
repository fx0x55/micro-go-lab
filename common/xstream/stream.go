package xstream

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fx0x55/micro-go-lab/common/xevent"
	"github.com/fx0x55/micro-go-lab/common/xmetrics"
	"github.com/segmentio/kafka-go"
	"github.com/zeromicro/go-zero/core/logx"
)

const (
	maxBackoff = 10 * time.Second
)

// Message 是 Kafka 消息的统一表示，与 broker SDK 解耦。
type Message struct {
	Topic     string
	Partition int
	Offset    int64
	Key       string
	Value     []byte
	Headers   map[string]string
}

// toKafkaHeaders 将 map[string]string 转换为 []kafka.Header。
func toKafkaHeaders(h map[string]string) []kafka.Header {
	result := make([]kafka.Header, 0, len(h))
	for k, v := range h {
		result = append(result, kafka.Header{Key: k, Value: []byte(v)})
	}
	return result
}

// fromKafkaHeaders 将 []kafka.Header 转换为 map[string]string。
func fromKafkaHeaders(h []kafka.Header) map[string]string {
	result := make(map[string]string, len(h))
	for _, hdr := range h {
		result[hdr.Key] = string(hdr.Value)
	}
	return result
}

// ---------------------------------------------------------------------------
// Producer
// ---------------------------------------------------------------------------

// Producer Kafka 同步生产者（封装 kafka.Writer）。
type Producer struct {
	writer *kafka.Writer
}

// NewProducer 创建 Kafka 生产者，并在启动时 ensure 所有 topic 存在。
func NewProducer(bootstrapServers []string) (*Producer, error) {
	if err := ensureTopics(bootstrapServers, xevent.TopicSpecs()); err != nil {
		return nil, fmt.Errorf("ensure topics: %w", err)
	}

	w := &kafka.Writer{
		Addr:         kafka.TCP(bootstrapServers...),
		Balancer:     &kafka.Hash{},
		RequiredAcks: kafka.RequireAll,
		MaxAttempts:  5,
		Async:        false,
		BatchTimeout: 10 * time.Millisecond,
	}

	return &Producer{writer: w}, nil
}

// Publish 同步写入一条消息到 Kafka topic。
func (p *Producer) Publish(ctx context.Context, msg *Message) error {
	return p.writer.WriteMessages(ctx, kafka.Message{
		Topic:   msg.Topic,
		Key:     []byte(msg.Key),
		Value:   msg.Value,
		Headers: toKafkaHeaders(msg.Headers),
	})
}

// Close 关闭底层 Writer。
func (p *Producer) Close() error {
	return p.writer.Close()
}

// ensureTopics 用 kafka-go 的 Admin API 创建 topic（已存在则忽略），并尽力设置 topic config。
func ensureTopics(bootstrapServers []string, specs []xevent.TopicSpec) error {
	if len(bootstrapServers) == 0 {
		return errors.New("no bootstrap servers")
	}

	// 逐个尝试连接 broker，直到成功（集群可能还在启动中）
	var conn *kafka.Conn
	var lastErr error
	for _, broker := range bootstrapServers {
		c, err := kafka.Dial("tcp", broker)
		if err != nil {
			lastErr = err
			continue
		}
		conn = c
		break
	}
	if conn == nil {
		return fmt.Errorf("dial brokers: %w", lastErr)
	}
	defer func() { _ = conn.Close() }()

	controller, err := conn.Controller()
	if err != nil {
		return fmt.Errorf("get controller: %w", err)
	}
	controllerAddr := net.JoinHostPort(controller.Host, strconv.Itoa(controller.Port))

	topicConn, err := kafka.Dial("tcp", controllerAddr)
	if err != nil {
		return fmt.Errorf("dial controller: %w", err)
	}
	defer func() { _ = topicConn.Close() }()

	for _, spec := range specs {
		err := topicConn.CreateTopics(kafka.TopicConfig{
			Topic:             spec.Name,
			NumPartitions:     spec.NumPartitions,
			ReplicationFactor: spec.ReplicationFactor,
		})
		if err != nil && !isTopicExistsErr(err) {
			logx.Error("create topic failed",
				logx.Field("topic", spec.Name),
				logx.Field("error", err.Error()))
		}
	}

	return nil
}

// isTopicExistsErr 检查错误是否为 "topic already exists"。
func isTopicExistsErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "already exists")
}

// ---------------------------------------------------------------------------
// Consumer
// ---------------------------------------------------------------------------

// ConsumerConfig 消费者配置
type ConsumerConfig struct {
	Brokers []string
	Topic   string
	Group   string
}

// Handler 消息处理函数
type Handler func(msg *Message) error

// Consumer Kafka 消费者（基于 kafka.Reader + 消费者组）
type Consumer struct {
	cfg     ConsumerConfig
	handler Handler
}

// NewConsumer 创建 Kafka 消费者
func NewConsumer(cfg ConsumerConfig, handler Handler) *Consumer {
	return &Consumer{
		cfg:     cfg,
		handler: handler,
	}
}

func consumerCaller(cfg ConsumerConfig) string {
	return "consumer:" + cfg.Group + "/" + cfg.Topic
}

// Start 启动消费者（在 goroutine 中运行）。
// ctx 取消时 goroutine 安全退出；wg 用于等待 goroutine 结束。
func (c *Consumer) Start(ctx context.Context, wg *sync.WaitGroup) {
	wg.Add(1)
	go c.consume(ctx, wg)
}

func (c *Consumer) consume(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	caller := consumerCaller(c.cfg)

	logx.Info("kafka consumer starting",
		logx.Field("topic", c.cfg.Topic),
		logx.Field("group", c.cfg.Group),
		logx.Field("brokers", c.cfg.Brokers))

	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:           c.cfg.Brokers,
		Topic:             c.cfg.Topic,
		GroupID:           c.cfg.Group,
		StartOffset:       kafka.FirstOffset,
		CommitInterval:    0, // 同步提交：处理成功后手动 CommitMessages
		SessionTimeout:    10 * time.Second,
		RebalanceTimeout:  10 * time.Second,
		HeartbeatInterval: 3 * time.Second,
		MinBytes:          1,
		MaxBytes:          10e6, // 10MB
	})
	defer func() { _ = r.Close() }()

	logx.Info("kafka consumer loop started",
		logx.Field("topic", c.cfg.Topic),
		logx.Field("group", c.cfg.Group))

	backoff := time.Second
	for {
		select {
		case <-ctx.Done():
			logx.Info("kafka consumer loop stopping",
				logx.Field("topic", c.cfg.Topic),
				logx.Field("group", c.cfg.Group))
			return
		default:
		}

		fetchErr := error(nil)
		RunWithRecover(ctx, caller, func(ctx context.Context) {
			msg, err := r.FetchMessage(ctx)
			if err != nil {
				fetchErr = err
				return
			}

			xMsg := &Message{
				Topic:     msg.Topic,
				Partition: msg.Partition,
				Offset:    msg.Offset,
				Key:       string(msg.Key),
				Value:     msg.Value,
				Headers:   fromKafkaHeaders(msg.Headers),
			}

			if err := c.handler(xMsg); err != nil {
				logx.Error("kafka handler failed",
					logx.Field("topic", msg.Topic),
					logx.Field("partition", msg.Partition),
					logx.Field("offset", msg.Offset),
					logx.Field("error", err.Error()))
				// 不提交 offset → at-least-once 会重投
				xmetrics.KafkaMessagesConsumed.WithLabelValues(
					c.cfg.Topic, c.cfg.Group, strconv.Itoa(msg.Partition), "error").Inc()
				return
			}

			// 同步提交 offset
			if err := r.CommitMessages(ctx, msg); err != nil {
				logx.Error("kafka commit failed",
					logx.Field("topic", msg.Topic),
					logx.Field("partition", msg.Partition),
					logx.Field("offset", msg.Offset),
					logx.Field("error", err.Error()))
			}

			xmetrics.KafkaMessagesConsumed.WithLabelValues(
				c.cfg.Topic, c.cfg.Group, strconv.Itoa(msg.Partition), "success").Inc()
		})

		if fetchErr != nil {
			if errors.Is(fetchErr, context.Canceled) {
				return
			}
			logx.Error("kafka fetch failed, retrying",
				logx.Field("topic", c.cfg.Topic),
				logx.Field("group", c.cfg.Group),
				logx.Field("error", fetchErr.Error()),
				logx.Field("backoff", backoff))

			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff = min(backoff*2, maxBackoff)
		} else {
			backoff = time.Second
		}
	}
}

package xstream

import (
	"crypto/rand"
	"math/big"
	"strconv"
	"time"

	"github.com/fx0x55/micro-go-lab/common/xevent"
	"github.com/fx0x55/micro-go-lab/common/xmetrics"
	"github.com/zeromicro/go-zero/core/logx"
)

// Poller Outbox 轮询器，定期从 DB 读取待发送事件并发布到 Redis Stream
type Poller struct {
	outboxRepo *xevent.OutboxRepository
	producer   *Producer
	interval   time.Duration
	batchSize  int
	done       chan struct{}
}

// NewPoller 创建 Outbox 轮询器
func NewPoller(outboxRepo *xevent.OutboxRepository, producer *Producer, interval time.Duration, batchSize int) *Poller {
	return &Poller{
		outboxRepo: outboxRepo,
		producer:   producer,
		interval:   interval,
		batchSize:  batchSize,
		done:       make(chan struct{}),
	}
}

// Start 启动轮询器（在 goroutine 中运行）
func (p *Poller) Start() {
	go p.poll()
}

func (p *Poller) poll() {
	// 首次 tick 加随机 jitter，防止多实例同时轮询造成 DB 抖动
	jitterBig, err := rand.Int(rand.Reader, big.NewInt(int64(p.interval)))
	if err == nil {
		time.Sleep(time.Duration(jitterBig.Int64()))
	}

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-p.done:
			return
		case <-ticker.C:
			p.tick()
		}
	}
}

func (p *Poller) tick() {
	events, err := p.outboxRepo.FindPending(p.batchSize)
	if err != nil {
		logx.Error("outbox find pending failed", logx.Field("error", err.Error()))
		return
	}

	for i := range events {
		event := &events[i]

		// 重试次数超限，标记为 failed 并跳过
		if event.RetryCount >= xevent.MaxRetries {
			logx.Error("outbox event max retries exceeded",
				logx.Field("event_id", event.EventID),
				logx.Field("retry_count", event.RetryCount))
			if err := p.outboxRepo.MarkAsFailed(event.ID, "max retries exceeded"); err != nil {
				logx.Error("outbox mark failed", logx.Field("id", event.ID), logx.Field("error", err.Error()))
			}
			continue
		}

		values := map[string]string{
			"event":       event.EventType,
			"event_id":    event.EventID,
			"event_key":   event.EventKey,
			"version":     strconv.Itoa(event.Version),
			"occurred_at": event.CreatedAt.Format(time.RFC3339),
			"payload":     event.Payload,
		}

		_, err := p.producer.Publish(event.Topic, values)
		if err != nil {
			logx.Error("outbox publish failed",
				logx.Field("event_id", event.EventID),
				logx.Field("stream", event.Topic),
				logx.Field("error", err.Error()),
			)
			if err := p.outboxRepo.IncrementRetryCount(event.ID, err.Error()); err != nil {
				logx.Error(
					"outbox increment retry failed",
					logx.Field("id", event.ID),
					logx.Field("error", err.Error()),
				)
			}
			xmetrics.OutboxEventsPublished.WithLabelValues(event.Topic, "error").Inc()
			continue
		}

		if err := p.outboxRepo.MarkAsSent(event.ID); err != nil {
			logx.Error("outbox mark sent failed", logx.Field("id", event.ID), logx.Field("error", err.Error()))
		}
		xmetrics.OutboxEventsPublished.WithLabelValues(event.Topic, "success").Inc()

		logx.Info("outbox event published",
			logx.Field("event_id", event.EventID),
			logx.Field("stream", event.Topic),
			logx.Field("event_type", event.EventType),
		)
	}
}

// Stop 停止轮询器
func (p *Poller) Stop() {
	close(p.done)
}

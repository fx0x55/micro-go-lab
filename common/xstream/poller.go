package xstream

import (
	"context"
	"crypto/rand"
	"math/big"
	"strconv"
	"sync"
	"time"

	"github.com/fx0x55/micro-go-lab/common/xevent"
	"github.com/fx0x55/micro-go-lab/common/xmetrics"
	"github.com/zeromicro/go-zero/core/logx"
)

// pollerCaller 用于日志标识 panic 发生在 poller goroutine。
const pollerCaller = "poller"

// Poller Outbox 轮询器，定期从 DB 读取待发送事件并发布到 Redis Stream
type Poller struct {
	outboxRepo *xevent.OutboxRepository
	producer   *Producer
	interval   time.Duration
	batchSize  int
}

// NewPoller 创建 Outbox 轮询器
func NewPoller(outboxRepo *xevent.OutboxRepository, producer *Producer, interval time.Duration, batchSize int) *Poller {
	return &Poller{
		outboxRepo: outboxRepo,
		producer:   producer,
		interval:   interval,
		batchSize:  batchSize,
	}
}

// Start 启动轮询器（在 goroutine 中运行）。
// ctx 取消时 goroutine 安全退出；wg 用于等待 goroutine 结束。
func (p *Poller) Start(ctx context.Context, wg *sync.WaitGroup) {
	wg.Add(1)
	go p.poll(ctx, wg)
}

func (p *Poller) poll(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	// 首次 tick 加随机 jitter，防止多实例同时轮询造成 DB 抖动
	jitterBig, err := rand.Int(rand.Reader, big.NewInt(int64(p.interval)))
	if err == nil {
		select {
		case <-time.After(time.Duration(jitterBig.Int64())):
		case <-ctx.Done():
			return
		}
	}

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			RunWithRecover(ctx, pollerCaller, func(ctx context.Context) {
				p.tick(ctx)
			})
		}
	}
}

func (p *Poller) tick(ctx context.Context) {
	events, err := p.outboxRepo.FindPending(p.batchSize)
	if err != nil {
		logx.Error("outbox find pending failed", logx.Field("error", err.Error()))
		return
	}

	for i := range events {
		event := &events[i]

		values := map[string]string{
			"event":       event.EventType,
			"event_id":    event.EventID,
			"event_key":   event.EventKey,
			"version":     strconv.Itoa(event.Version),
			"occurred_at": event.CreatedAt.Format(time.RFC3339),
			"payload":     event.Payload,
		}

		_, err := p.producer.Publish(ctx, event.Topic, values)
		if err != nil {
			logx.Error("outbox publish failed",
				logx.Field("event_id", event.EventID),
				logx.Field("stream", event.Topic),
				logx.Field("error", err.Error()),
			)
			if err := p.outboxRepo.IncrementRetryCount(event.ID, err.Error()); err != nil {
				logx.WithContext(ctx).Error(
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

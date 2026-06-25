package svc

import (
	"encoding/json"

	"github.com/fx0x55/micro-go-lab/common/xevent"
	"github.com/fx0x55/micro-go-lab/common/xmetrics"
	"github.com/fx0x55/micro-go-lab/common/xstream"
	"github.com/zeromicro/go-zero/core/logx"
	"gorm.io/gorm"
)

// HandleOrderEvent 消费 order-events stream 的消息（带幂等）
func HandleOrderEvent(idempotentRepo *xevent.IdempotentRepository) xstream.Handler {
	return func(values map[string]string) error {
		eventID := values["event_id"]
		if eventID == "" {
			return nil
		}

		payload := values["payload"]
		var envelope xevent.Envelope
		if err := json.Unmarshal([]byte(payload), &envelope); err != nil {
			logx.Error("unmarshal order event failed",
				logx.Field("event_id", eventID),
				logx.Field("error", err.Error()))
			xmetrics.RedisStreamMessagesConsumed.WithLabelValues(xevent.TopicOrderEvents, "user-api", "error").Inc()
			return nil // 反序列化失败是永久性错误，跳过避免无限重试
		}

		processed, err := idempotentRepo.Process(eventID, func(tx *gorm.DB) error {
			return handleOrderEvent(&envelope)
		})
		if err != nil {
			logx.Error("event processing failed",
				logx.Field("event_id", eventID),
				logx.Field("error", err.Error()))
			return err
		}

		if !processed {
			logx.Info("event already processed, skipping", logx.Field("event_id", eventID))
		}

		return nil
	}
}

func handleOrderEvent(envelope *xevent.Envelope) error {
	switch envelope.Event {
	case xevent.OrderCreated:
		data, _ := json.Marshal(envelope.Data)
		logx.Info("received OrderCreated",
			logx.Field("event", string(envelope.Event)),
			logx.Field("data", string(data)))
	default:
		logx.Info("received unknown event", logx.Field("event", string(envelope.Event)))
	}
	return nil
}

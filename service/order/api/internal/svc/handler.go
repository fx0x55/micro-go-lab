package svc

import (
	"encoding/json"

	"github.com/fx0x55/micro-go-lab/common/xevent"
	"github.com/fx0x55/micro-go-lab/common/xmetrics"
	"github.com/zeromicro/go-zero/core/logx"
	"gorm.io/gorm"
)

// HandleUserEvent 消费 user-events stream 的消息（带幂等）
func HandleUserEvent(db *gorm.DB) func(values map[string]string) error {
	return func(values map[string]string) error {
		eventID := values["event_id"]
		if eventID == "" {
			return nil
		}

		// 幂等：检查是否已处理
		var exists bool
		db.Raw("SELECT EXISTS(SELECT 1 FROM processed_events WHERE event_id = ?)", eventID).Scan(&exists)
		if exists {
			return nil
		}

		payload := values["payload"]
		var envelope xevent.Envelope
		if err := json.Unmarshal([]byte(payload), &envelope); err != nil {
			logx.Error("unmarshal user event failed", logx.Field("error", err.Error()))
			xmetrics.RedisStreamMessagesConsumed.WithLabelValues(xevent.TopicUserEvents, "order-api", "error").Inc()
			return err
		}

		switch envelope.Event {
		case xevent.UserRegistered:
			data, _ := json.Marshal(envelope.Data)
			logx.Info("received UserRegistered",
				logx.Field("event", string(envelope.Event)),
				logx.Field("data", string(data)),
			)
		default:
			logx.Info("received unknown event", logx.Field("event", string(envelope.Event)))
		}

		// 标记已处理
		db.Exec("INSERT INTO processed_events (event_id) VALUES (?) ON CONFLICT DO NOTHING", eventID)
		return nil
	}
}

package svc

import (
	"encoding/json"

	"github.com/fx0x55/micro-go-lab/common/xevent"
	"github.com/fx0x55/micro-go-lab/common/xmetrics"
	"github.com/zeromicro/go-zero/core/logx"
)

// HandleUserEvent 消费 user-events stream 的消息
func HandleUserEvent(values map[string]string) error {
	payload := values["payload"]
	var envelope xevent.Envelope
	if err := json.Unmarshal([]byte(payload), &envelope); err != nil {
		logx.Error("unmarshal user event failed", logx.Field("error", err.Error()))
		xmetrics.KafkaMessagesConsumed.WithLabelValues(xevent.TopicUserEvents, "order-api", "error").Inc()
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

	return nil
}

package svc

import (
	"encoding/json"

	"github.com/fx0x55/micro-go-lab/common/xevent"
	"github.com/fx0x55/micro-go-lab/common/xmetrics"
	"github.com/zeromicro/go-zero/core/logx"
)

// HandleOrderEvent 消费 order-events stream 的消息
func HandleOrderEvent(values map[string]string) error {
	payload := values["payload"]
	var envelope xevent.Envelope
	if err := json.Unmarshal([]byte(payload), &envelope); err != nil {
		logx.Error("unmarshal order event failed", logx.Field("error", err.Error()))
		xmetrics.KafkaMessagesConsumed.WithLabelValues(xevent.TopicOrderEvents, "user-api", "error").Inc()
		return err
	}

	switch envelope.Event {
	case xevent.OrderCreated:
		data, _ := json.Marshal(envelope.Data)
		logx.Info("received OrderCreated",
			logx.Field("event", string(envelope.Event)),
			logx.Field("data", string(data)),
		)
	default:
		logx.Info("received unknown event", logx.Field("event", string(envelope.Event)))
	}

	return nil
}

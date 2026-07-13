package svc

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/fx0x55/micro-go-lab/common/xevent"
	"github.com/fx0x55/micro-go-lab/common/xmetrics"
	"github.com/fx0x55/micro-go-lab/common/xstream"
	"github.com/fx0x55/micro-go-lab/service/order/api/internal/model"
	"github.com/zeromicro/go-zero/core/logx"
	"gorm.io/gorm"
)

// handleUserEvent 处理 user-events Kafka 消息。
// 解析 Envelope → 取 event_id header → 通过 IdempotentRepository 去重 →
// 更新 known_users 物化视图（CQRS）。
func (sc *ServiceContext) handleUserEvent(msg *xstream.Message) error {
	eventID := msg.Headers["event_id"]
	if eventID == "" {
		logx.Error("user-event missing event_id header")
		return nil
	}

	var envelope xevent.Envelope
	if err := json.Unmarshal(msg.Value, &envelope); err != nil {
		logx.WithContext(sc.ctx).Error("failed to unmarshal user-event envelope",
			logx.Field("event_id", eventID),
			logx.Field("error", err.Error()))
		return err
	}

	processed, err := sc.IdempotentRepo.Process(eventID, func(tx *gorm.DB) error {
		data, dErr := json.Marshal(envelope.Data)
		if dErr != nil {
			return dErr
		}
		var userData xevent.UserRegisteredData
		if uErr := json.Unmarshal(data, &userData); uErr != nil {
			return uErr
		}

		ku := &model.KnownUser{
			UserID:   userData.UserID,
			Username: userData.Username,
		}
		return model.UpsertKnownUser(tx, ku)
	})
	if err != nil {
		logx.WithContext(sc.ctx).Error("user-event processing failed",
			logx.Field("event_id", eventID),
			logx.Field("error", err.Error()))
		xmetrics.KafkaMessagesConsumed.WithLabelValues(
			sc.Config.Kafka.Topic, sc.Config.Kafka.GroupID, strconv.Itoa(msg.Partition), "error").Inc()
		return err
	}

	if !processed {
		logx.WithContext(sc.ctx).Info("user-event skipped (already processed)",
			logx.Field("event_id", eventID))
		xmetrics.KafkaMessagesConsumed.WithLabelValues(
			sc.Config.Kafka.Topic, sc.Config.Kafka.GroupID, strconv.Itoa(msg.Partition), "skip").Inc()
		return nil
	}

	logx.WithContext(sc.ctx).Info("user-event processed, known_users upserted",
		logx.Field("event_id", eventID),
		logx.Field("event_type", fmt.Sprint(envelope.Event)))
	xmetrics.KafkaMessagesConsumed.WithLabelValues(
		sc.Config.Kafka.Topic, sc.Config.Kafka.GroupID, strconv.Itoa(msg.Partition), "success").Inc()
	return nil
}

package svc

import (
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/fx0x55/micro-go-lab/common/xevent"
	"github.com/fx0x55/micro-go-lab/common/xmetrics"
	"github.com/fx0x55/micro-go-lab/common/xstream"
	"github.com/fx0x55/micro-go-lab/service/order/api/internal/model"
	"github.com/google/uuid"
	"github.com/zeromicro/go-zero/core/logx"
	"gorm.io/gorm"
)

// handleInventoryEvent 处理 inventory-events Kafka 消息。
// InventoryReserved -> no-op（订单保持 pending）。
// ReservationFailed -> 自动取消订单（pending->cancelled, cancel_reason=OUT_OF_STOCK）
//   - 写 outbox[OrderCancelled] 触发库存释放。
func (sc *ServiceContext) handleInventoryEvent(msg *xstream.Message) error {
	eventID := msg.Headers["event_id"]
	if eventID == "" {
		logx.Error("inventory-event missing event_id header")
		return nil
	}

	var envelope xevent.Envelope
	if err := json.Unmarshal(msg.Value, &envelope); err != nil {
		logx.Errorf("failed to unmarshal inventory-event envelope: %v", err)
		return err
	}

	processed, err := sc.IdempotentRepo.Process(eventID, func(tx *gorm.DB) error {
		data, dErr := json.Marshal(envelope.Data)
		if dErr != nil {
			return dErr
		}

		switch envelope.Event {
		case xevent.ReservationFailed:
			return sc.handleReservationFailed(tx, data)
		case xevent.InventoryReserved:
			// 预占成功，订单保持 pending，no-op。
			return nil
		default:
			logx.Infof("ignoring inventory-event type %s", envelope.Event)
			return nil
		}
	})
	if err != nil {
		logx.Errorf("inventory-event processing failed: type=%s event_id=%s err=%v",
			envelope.Event, eventID, err)
		xmetrics.KafkaMessagesConsumed.WithLabelValues(
			sc.Config.Kafka.InventoryTopic, sc.Config.Kafka.GroupID,
			strconv.Itoa(msg.Partition), "error").Inc()
		return err
	}
	if !processed {
		logx.Infof("inventory-event skipped (already processed): event_id=%s", eventID)
		return nil
	}

	logx.Infof("inventory-event processed: type=%s event_id=%s", envelope.Event, eventID)
	return nil
}

// handleReservationFailed 缺货自动取消订单，并写 outbox[OrderCancelled] 通知库存。
// 若订单已被用户抢先取消（已是 cancelled），no-op（幂等）。
func (sc *ServiceContext) handleReservationFailed(tx *gorm.DB, data []byte) error {
	var d xevent.ReservationFailedData
	if err := json.Unmarshal(data, &d); err != nil {
		return err
	}

	var order model.Order
	err := tx.Where("id = ?", d.OrderID).First(&order).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			logx.Infof("ReservationFailed no-op: order_id=%d not found", d.OrderID)
			return nil
		}
		return err
	}

	// 用户已抢先取消，no-op（幂等，竞态闭合）。
	if order.Status == model.StatusCancelled {
		logx.Infof("ReservationFailed no-op: order_id=%d already cancelled", d.OrderID)
		return nil
	}

	affected, ord, err := sc.OrderRepo.Cancel(tx, order.UserID, order.ID, order.Version, xevent.ReasonOutOfStock)
	if err != nil {
		return err
	}
	if affected == 0 {
		// 并发取消冲突，视为已处理。
		logx.Infof("ReservationFailed cancel conflict: order_id=%d", d.OrderID)
		return nil
	}

	payload, err := json.Marshal(xevent.Envelope{
		Event:      xevent.OrderCancelled,
		Version:    1,
		OccurredAt: time.Now(),
		Data: xevent.OrderCancelledData{
			OrderID:  ord.ID,
			Sku:      ord.Sku,
			Quantity: ord.Quantity,
			Reason:   xevent.ReasonOutOfStock,
		},
	})
	if err != nil {
		return err
	}
	outboxEvent := &xevent.OutboxEvent{
		EventID:   uuid.New().String(),
		Topic:     xevent.TopicOrderEvents,
		EventKey:  strconv.FormatUint(uint64(ord.ID), 10),
		EventType: string(xevent.OrderCancelled),
		Version:   1,
		Payload:   string(payload),
		Status:    xevent.OutboxStatusPending,
	}
	return sc.OutboxRepo.Insert(tx, outboxEvent)
}

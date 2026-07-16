package svc

import (
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/fx0x55/micro-go-lab/common/xevent"
	"github.com/fx0x55/micro-go-lab/common/xmetrics"
	"github.com/fx0x55/micro-go-lab/common/xstream"
	"github.com/google/uuid"
	"github.com/zeromicro/go-zero/core/logx"
	"gorm.io/gorm"
)

// handleOrderEvent 处理 order-events Kafka 消息。
// 解析 Envelope -> 取 event_id header -> 通过 IdempotentRepository 去重 ->
// 根据 event 类型执行预占（OrderCreated）或释放（OrderCancelled）。
func (sc *ServiceContext) handleOrderEvent(msg *xstream.Message) error {
	eventID := msg.Headers["event_id"]
	if eventID == "" {
		logx.Error("order-event missing event_id header")
		return nil
	}

	var envelope xevent.Envelope
	if err := json.Unmarshal(msg.Value, &envelope); err != nil {
		logx.Errorf("failed to unmarshal order-event envelope: %v", err)
		return err
	}

	processed, err := sc.IdempotentRepo.Process(eventID, func(tx *gorm.DB) error {
		data, dErr := json.Marshal(envelope.Data)
		if dErr != nil {
			return dErr
		}

		switch envelope.Event {
		case xevent.OrderCreated:
			return sc.handleOrderCreated(tx, data)
		case xevent.OrderCancelled:
			return sc.handleOrderCancelled(tx, data)
		default:
			logx.Infof("ignoring order-event type %s", envelope.Event)
			return nil
		}
	})
	if err != nil {
		logx.Errorf("order-event processing failed: type=%s event_id=%s err=%v",
			envelope.Event, eventID, err)
		xmetrics.KafkaMessagesConsumed.WithLabelValues(
			sc.Config.Kafka.Topic, sc.Config.Kafka.GroupID,
			strconv.Itoa(msg.Partition), "error").Inc()
		return err
	}
	if !processed {
		logx.Infof("order-event skipped (already processed): event_id=%s", eventID)
		return nil
	}

	logx.Infof("order-event processed: type=%s event_id=%s", envelope.Event, eventID)
	return nil
}

// handleOrderCreated 预占库存：原子扣减 available + 插 reservation + outbox。
// 库存不足时发 ReservationFailed；成功时发 InventoryReserved。
func (sc *ServiceContext) handleOrderCreated(tx *gorm.DB, data []byte) error {
	var d xevent.OrderCreatedData
	if err := json.Unmarshal(data, &d); err != nil {
		return err
	}

	affected, err := sc.InventoryRepo.Reserve(tx, d.OrderID, d.Sku, d.Quantity)
	if err != nil {
		return err
	}

	var eventType xevent.EventType
	var payload any
	if affected == 1 {
		eventType = xevent.InventoryReserved
		payload = xevent.InventoryReservedData{OrderID: d.OrderID}
	} else {
		eventType = xevent.ReservationFailed
		payload = xevent.ReservationFailedData{
			OrderID: d.OrderID,
			Reason:  xevent.ReasonOutOfStock,
		}
	}

	return sc.writeOutbox(tx, eventType, d.OrderID, payload)
}

// handleOrderCancelled 释放预占：翻 reservation status->released + 回补 available。
// 若无 active reservation（未预占或已释放），为 no-op，仅记录日志。
func (sc *ServiceContext) handleOrderCancelled(tx *gorm.DB, data []byte) error {
	var d xevent.OrderCancelledData
	if err := json.Unmarshal(data, &d); err != nil {
		return err
	}

	affected, err := sc.InventoryRepo.Release(tx, d.OrderID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// 预占从未成功或已被释放，no-op（幂等）。
			logx.Infof("OrderCancelled no-op: order_id=%d no active reservation", d.OrderID)
			return nil
		}
		return err
	}
	if affected == 0 {
		logx.Infof("OrderCancelled no-op: order_id=%d reservation already released", d.OrderID)
	}
	return nil
}

// writeOutbox 在事务内写入一条 outbox 事件，发往 inventory-events topic。
func (sc *ServiceContext) writeOutbox(
	tx *gorm.DB, event xevent.EventType, orderID uint, data any,
) error {
	payload, err := json.Marshal(xevent.Envelope{
		Event:      event,
		Version:    1,
		OccurredAt: time.Now(),
		Data:       data,
	})
	if err != nil {
		return err
	}
	outboxEvent := &xevent.OutboxEvent{
		EventID:   uuid.New().String(),
		Topic:     xevent.TopicInventoryEvents,
		EventKey:  strconv.FormatUint(uint64(orderID), 10),
		EventType: string(event),
		Version:   1,
		Payload:   string(payload),
		Status:    xevent.OutboxStatusPending,
	}
	return sc.OutboxRepo.Insert(tx, outboxEvent)
}

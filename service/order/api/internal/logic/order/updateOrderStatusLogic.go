package order

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/fx0x55/micro-go-lab/common/ecode"
	"github.com/fx0x55/micro-go-lab/common/xevent"
	"github.com/fx0x55/micro-go-lab/service/order/api/internal/model"
	"github.com/fx0x55/micro-go-lab/service/order/api/internal/svc"
	"github.com/fx0x55/micro-go-lab/service/order/api/internal/types"
	"github.com/google/uuid"
	"github.com/zeromicro/go-zero/core/logx"
	"gorm.io/gorm"
)

var (
	ErrOrderNotFound           = ecode.ErrOrderNotFound
	ErrInvalidStatusTransition = ecode.ErrInvalidStatusTransition
)

type UpdateOrderStatusLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpdateOrderStatusLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateOrderStatusLogic {
	return &UpdateOrderStatusLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UpdateOrderStatusLogic) UpdateStatus(
	userID, id uint,
	req *types.UpdateOrderStatusRequest,
) (*model.Order, error) {
	order, err := l.svcCtx.OrderRepo.FindByIDAndUserID(l.ctx, id, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOrderNotFound
		}
		return nil, err
	}

	if !isValidTransition(order.Status, req.Status) {
		return nil, ErrInvalidStatusTransition
	}

	if req.Status == model.StatusCancelled {
		return l.cancelOrder(order, xevent.ReasonUserCancelled)
	}
	return l.paidOrder(order)
}

// cancelOrder 在事务内取消订单 + 写 outbox[OrderCancelled]，
// 使 inventory 消费后释放预占库存。
func (l *UpdateOrderStatusLogic) cancelOrder(order *model.Order, reason string) (*model.Order, error) {
	var cancelled *model.Order

	err := l.svcCtx.DB.Transaction(func(tx *gorm.DB) error {
		affected, ord, err := l.svcCtx.OrderRepo.Cancel(tx, order.UserID, order.ID, order.Version, reason)
		if err != nil {
			return err
		}
		if affected == 0 {
			return ecode.ErrOptimisticConflict
		}
		cancelled = ord

		payload, err := json.Marshal(xevent.Envelope{
			Event:      xevent.OrderCancelled,
			Version:    1,
			OccurredAt: time.Now(),
			Data: xevent.OrderCancelledData{
				OrderID:  cancelled.ID,
				Sku:      cancelled.Sku,
				Quantity: cancelled.Quantity,
				Reason:   reason,
			},
		})
		if err != nil {
			return err
		}
		outboxEvent := &xevent.OutboxEvent{
			EventID:   uuid.New().String(),
			Topic:     xevent.TopicOrderEvents,
			EventKey:  strconv.FormatUint(uint64(cancelled.ID), 10),
			EventType: string(xevent.OrderCancelled),
			Version:   1,
			Payload:   string(payload),
			Status:    xevent.OutboxStatusPending,
		}
		return l.svcCtx.OutboxRepo.Insert(tx, outboxEvent)
	})
	if err != nil {
		return nil, err
	}

	return cancelled, nil
}

func (l *UpdateOrderStatusLogic) paidOrder(order *model.Order) (*model.Order, error) {
	affected, err := l.svcCtx.OrderRepo.UpdateStatus(
		l.ctx, order.UserID, order.ID, order.Status, model.StatusPaid, order.Version)
	if err != nil {
		return nil, err
	}
	if affected == 0 {
		return nil, ecode.ErrOptimisticConflict
	}

	order.Status = model.StatusPaid
	return order, nil
}

package order

import (
	"context"
	"errors"

	"github.com/fx0x55/micro-go-lab/common/ecode"
	"github.com/fx0x55/micro-go-lab/service/order/api/internal/model"
	"github.com/fx0x55/micro-go-lab/service/order/api/internal/svc"
	"github.com/fx0x55/micro-go-lab/service/order/api/internal/types"
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

	affected, err := l.svcCtx.OrderRepo.UpdateStatus(l.ctx, userID, id, order.Status, req.Status, order.Version)
	if err != nil {
		return nil, err
	}
	if affected == 0 {
		return nil, ecode.ErrOptimisticConflict
	}

	order.Status = req.Status
	return order, nil
}

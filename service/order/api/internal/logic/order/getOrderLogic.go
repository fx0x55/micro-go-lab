package order

import (
	"context"
	"errors"

	"github.com/fx0x55/micro-go-lab/common/ecode"
	"github.com/fx0x55/micro-go-lab/service/order/api/internal/model"
	"github.com/fx0x55/micro-go-lab/service/order/api/internal/svc"
	"github.com/zeromicro/go-zero/core/logx"
	"gorm.io/gorm"
)

type GetOrderLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetOrderLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetOrderLogic {
	return &GetOrderLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetOrderLogic) GetByID(userID, id uint) (*model.Order, error) {
	order, err := l.svcCtx.OrderRepo.FindByIDAndUserID(l.ctx, id, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ecode.ErrOrderNotFound
		}
		return nil, err
	}
	return order, nil
}

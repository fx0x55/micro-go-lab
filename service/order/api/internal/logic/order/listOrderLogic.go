package order

import (
	"context"

	"github.com/fx0x55/micro-go-lab/common/page"
	"github.com/fx0x55/micro-go-lab/service/order/api/internal/svc"
	"github.com/zeromicro/go-zero/core/logx"
)

type ListOrderLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListOrderLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListOrderLogic {
	return &ListOrderLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListOrderLogic) ListByUserID(userID uint, pg, pageSize int) (*page.Result, error) {
	orders, total, err := l.svcCtx.OrderRepo.FindByUserIDWithPage(l.ctx, userID, (pg-1)*pageSize, pageSize)
	if err != nil {
		return nil, err
	}
	return page.NewResult(orders, total, pg, pageSize), nil
}

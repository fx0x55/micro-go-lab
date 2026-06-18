package logic

import (
	"context"
	"errors"

	"github.com/zeromicro/go-zero/core/logx"
	"gorm.io/gorm"

	"github.com/wokoworks/go-server/common/page"
	"github.com/wokoworks/go-server/common/ecode"
	"github.com/wokoworks/go-server/common/model"
	"github.com/wokoworks/go-server/service/order/api/internal/svc"
	"github.com/wokoworks/go-server/service/order/api/internal/types"
)

// 保留向后兼容的别名
var (
	ErrOrderNotFound           = ecode.ErrOrderNotFound
	ErrUserNotFound            = ecode.ErrUserNotFound
	ErrInvalidStatusTransition = ecode.ErrInvalidStatusTransition
)

var validTransitions = map[string]map[string]bool{
	model.StatusPending:   {model.StatusPaid: true, model.StatusCancelled: true},
	model.StatusPaid:      {},
	model.StatusCancelled: {},
}

type CreateOrderLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateOrderLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateOrderLogic {
	return &CreateOrderLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateOrderLogic) Create(userID uint, req *types.CreateOrderRequest) (*model.Order, error) {
	summary, err := l.svcCtx.UserCli.ValidateUser(l.ctx, userID)
	if err != nil {
		return nil, err
	}
	if !summary.Exists {
		return nil, ErrUserNotFound
	}

	order := &model.Order{
		UserID:      userID,
		ProductName: req.ProductName,
		Amount:      req.Amount,
		Status:      model.StatusPending,
	}
	if err := l.svcCtx.OrderRepo.Create(order); err != nil {
		return nil, err
	}
	return order, nil
}

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
	order, err := l.svcCtx.OrderRepo.FindByIDAndUserID(id, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOrderNotFound
		}
		return nil, err
	}
	return order, nil
}

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
	orders, total, err := l.svcCtx.OrderRepo.FindByUserIDWithPage(userID, (pg-1)*pageSize, pageSize)
	if err != nil {
		return nil, err
	}
	return page.NewResult(orders, total, pg, pageSize), nil
}

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

func (l *UpdateOrderStatusLogic) UpdateStatus(userID, id uint, req *types.UpdateOrderStatusRequest) (*model.Order, error) {
	order, err := l.svcCtx.OrderRepo.FindByIDAndUserID(id, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOrderNotFound
		}
		return nil, err
	}

	if !isValidTransition(order.Status, req.Status) {
		return nil, ErrInvalidStatusTransition
	}

	order.Status = req.Status
	if err := l.svcCtx.OrderRepo.Update(order); err != nil {
		return nil, err
	}
	return order, nil
}

func isValidTransition(from, to string) bool {
	allowed, ok := validTransitions[from]
	if !ok {
		return false
	}
	return allowed[to]
}

package service

import (
	"context"
	"errors"

	"github.com/wokoworks/go-server/internal/client"
	"github.com/wokoworks/go-server/internal/order/model"
	"github.com/wokoworks/go-server/internal/order/repository"
	"gorm.io/gorm"
)

var (
	ErrOrderNotFound           = errors.New("order not found")
	ErrUserNotFound            = errors.New("user does not exist")
	ErrInvalidStatusTransition = errors.New("invalid status transition")
)

// validTransitions 定义订单状态允许的迁移路径
var validTransitions = map[string]map[string]bool{
	model.StatusPending:   {model.StatusPaid: true, model.StatusCancelled: true},
	model.StatusPaid:      {},
	model.StatusCancelled: {},
}

type OrderService struct {
	orderRepo *repository.OrderRepository
	userCli   *client.UserClient
}

func NewOrderService(orderRepo *repository.OrderRepository, userCli *client.UserClient) *OrderService {
	return &OrderService{orderRepo: orderRepo, userCli: userCli}
}

type CreateOrderRequest struct {
	ProductName string `json:"product_name" validate:"required,min=1,max=256"`
	Amount      int64  `json:"amount" validate:"required,gt=0"`
}

type UpdateOrderStatusRequest struct {
	Status string `json:"status" validate:"required,oneof=paid cancelled"`
}

func (s *OrderService) Create(ctx context.Context, userID uint, req *CreateOrderRequest) (*model.Order, error) {
	// 通过 gRPC 验证用户是否存在
	summary, err := s.userCli.ValidateUser(ctx, userID)
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
	if err := s.orderRepo.Create(order); err != nil {
		return nil, err
	}
	return order, nil
}

func (s *OrderService) GetByID(userID, id uint) (*model.Order, error) {
	order, err := s.orderRepo.FindByIDAndUserID(id, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOrderNotFound
		}
		return nil, err
	}
	return order, nil
}

func (s *OrderService) ListByUserID(userID uint) ([]model.Order, error) {
	return s.orderRepo.FindByUserID(userID)
}

func (s *OrderService) UpdateStatus(userID, id uint, req *UpdateOrderStatusRequest) (*model.Order, error) {
	order, err := s.orderRepo.FindByIDAndUserID(id, userID)
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
	if err := s.orderRepo.Update(order); err != nil {
		return nil, err
	}
	return order, nil
}

// isValidTransition 校验状态迁移是否在合法路径内
func isValidTransition(from, to string) bool {
	allowed, ok := validTransitions[from]
	if !ok {
		return false
	}
	return allowed[to]
}

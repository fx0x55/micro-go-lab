package repository

import (
	"context"

	"github.com/fx0x55/micro-go-lab/service/order/api/internal/model"
	"gorm.io/gorm"
)

// OrderRepositoryInterface 定义订单数据访问层的接口
type OrderRepositoryInterface interface {
	Create(tx *gorm.DB, order *model.Order) error
	FindByIDAndUserID(ctx context.Context, id, userID uint) (*model.Order, error)
	FindByUserID(ctx context.Context, userID uint) ([]model.Order, error)
	FindByUserIDWithPage(ctx context.Context, userID uint, offset, limit int) ([]model.Order, int64, error)
	UpdateStatus(ctx context.Context, userID, id uint, fromStatus, toStatus string, version int) (int64, error)
	// Cancel 在事务内取消订单（乐观锁 + cancel_reason），用于触发 saga 补偿。
	// 返回取消后的订单（含 Sku/Quantity），供调用方构建 outbox 事件。
	Cancel(tx *gorm.DB, userID, id uint, version int, reason string) (int64, *model.Order, error)
}

type OrderRepository struct {
	db *gorm.DB
}

func NewOrderRepository(db *gorm.DB) *OrderRepository {
	return &OrderRepository{db: db}
}

func (r *OrderRepository) Create(tx *gorm.DB, order *model.Order) error {
	return tx.Create(order).Error
}

func (r *OrderRepository) FindByIDAndUserID(ctx context.Context, id, userID uint) (*model.Order, error) {
	var order model.Order
	err := r.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).First(&order).Error
	if err != nil {
		return nil, err
	}
	return &order, nil
}

func (r *OrderRepository) FindByUserID(ctx context.Context, userID uint) ([]model.Order, error) {
	var orders []model.Order
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).Order("id DESC").Find(&orders).Error
	return orders, err
}

func (r *OrderRepository) FindByUserIDWithPage(
	ctx context.Context,
	userID uint,
	offset, limit int,
) ([]model.Order, int64, error) {
	var orders []model.Order
	var total int64

	db := r.db.WithContext(ctx).Model(&model.Order{}).Where("user_id = ?", userID)
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := db.Order("id DESC").Offset(offset).Limit(limit).Find(&orders).Error
	return orders, total, err
}

func (r *OrderRepository) UpdateStatus(
	ctx context.Context,
	userID, id uint,
	fromStatus, toStatus string,
	version int,
) (int64, error) {
	result := r.db.WithContext(ctx).
		Model(&model.Order{}).
		Where("id = ? AND user_id = ? AND status = ? AND version = ?", id, userID, fromStatus, version).
		Updates(map[string]any{
			"status":  toStatus,
			"version": gorm.Expr("version + 1"),
		})
	return result.RowsAffected, result.Error
}

func (r *OrderRepository) Cancel(
	tx *gorm.DB,
	userID, id uint,
	version int,
	reason string,
) (int64, *model.Order, error) {
	result := tx.
		Model(&model.Order{}).
		Where("id = ? AND user_id = ? AND status = ? AND version = ?", id, userID, model.StatusPending, version).
		Updates(map[string]any{
			"status":        model.StatusCancelled,
			"version":       gorm.Expr("version + 1"),
			"cancel_reason": reason,
		})
	if result.Error != nil {
		return 0, nil, result.Error
	}
	if result.RowsAffected == 0 {
		return 0, nil, nil
	}

	var order model.Order
	if err := tx.Where("id = ?", id).First(&order).Error; err != nil {
		return 0, nil, err
	}
	return result.RowsAffected, &order, nil
}

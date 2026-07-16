package repository

import (
	"context"

	"github.com/fx0x55/micro-go-lab/service/inventory/rpc/internal/model"
	"gorm.io/gorm"
)

// InventoryRepositoryInterface 定义库存数据访问层的接口。
type InventoryRepositoryInterface interface {
	GetProduct(ctx context.Context, sku string) (*model.Product, error)
	// Reserve 在给定事务内预占库存：原子扣减 available + 插 reservation。
	// available 不足时扣减 affected rows=0，调用方据此判断缺货。
	Reserve(tx *gorm.DB, orderID uint, sku string, qty int) (int64, error)
	// FindReservationByOrderID 查找订单对应的预占记录。
	FindReservationByOrderID(ctx context.Context, orderID uint) (*model.Reservation, error)
	// Release 在给定事务内释放预占：翻 reservation status->released + 回补 available。
	// 顺序：先翻 status，再回补 available（顺序不可反）。
	Release(tx *gorm.DB, orderID uint) (int64, error)
}

type InventoryRepository struct {
	db *gorm.DB
}

func NewInventoryRepository(db *gorm.DB) *InventoryRepository {
	return &InventoryRepository{db: db}
}

func (r *InventoryRepository) GetProduct(ctx context.Context, sku string) (*model.Product, error) {
	var p model.Product
	err := r.db.WithContext(ctx).Where("sku = ?", sku).First(&p).Error
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *InventoryRepository) Reserve(tx *gorm.DB, orderID uint, sku string, qty int) (int64, error) {
	result := tx.Model(&model.Product{}).
		Where("sku = ? AND available >= ?", sku, qty).
		Update("available", gorm.Expr("available - ?", qty))
	if result.Error != nil {
		return 0, result.Error
	}
	if result.RowsAffected == 0 {
		return 0, nil
	}

	reservation := &model.Reservation{
		OrderID:  orderID,
		Sku:      sku,
		Quantity: qty,
		Status:   model.ReservationStatusReserved,
	}
	if err := tx.Create(reservation).Error; err != nil {
		return 0, err
	}
	return 1, nil
}

func (r *InventoryRepository) FindReservationByOrderID(
	ctx context.Context, orderID uint,
) (*model.Reservation, error) {
	var res model.Reservation
	err := r.db.WithContext(ctx).Where("order_id = ?", orderID).First(&res).Error
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// Release 先翻 reservation status，再回补 available。
// 仅对 status=reserved 的记录生效；已 released 则 affected=0（no-op，幂等）。
func (r *InventoryRepository) Release(tx *gorm.DB, orderID uint) (int64, error) {
	var res model.Reservation
	if err := tx.Where("order_id = ?", orderID).First(&res).Error; err != nil {
		return 0, err
	}
	if res.Status == model.ReservationStatusReleased {
		return 0, nil
	}

	if err := tx.Model(&model.Reservation{}).
		Where("order_id = ? AND status = ?", orderID, model.ReservationStatusReserved).
		Update("status", model.ReservationStatusReleased).Error; err != nil {
		return 0, err
	}

	if err := tx.Model(&model.Product{}).
		Where("sku = ?", res.Sku).
		Update("available", gorm.Expr("available + ?", res.Quantity)).Error; err != nil {
		return 0, err
	}
	return 1, nil
}

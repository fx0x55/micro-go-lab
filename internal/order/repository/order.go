package repository

import (
	"github.com/wokoworks/go-server/internal/order/model"
	"gorm.io/gorm"
)

type OrderRepository struct {
	db *gorm.DB
}

func NewOrderRepository(db *gorm.DB) *OrderRepository {
	return &OrderRepository{db: db}
}

func (r *OrderRepository) Create(order *model.Order) error {
	return r.db.Create(order).Error
}

func (r *OrderRepository) FindByIDAndUserID(id, userID uint) (*model.Order, error) {
	var order model.Order
	err := r.db.Where("id = ? AND user_id = ?", id, userID).First(&order).Error
	if err != nil {
		return nil, err
	}
	return &order, nil
}

func (r *OrderRepository) FindByUserID(userID uint) ([]model.Order, error) {
	var orders []model.Order
	err := r.db.Where("user_id = ?", userID).Order("created_at DESC").Find(&orders).Error
	return orders, err
}

func (r *OrderRepository) Update(order *model.Order) error {
	return r.db.Save(order).Error
}

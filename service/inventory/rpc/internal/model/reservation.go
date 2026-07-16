package model

import "time"

// Reservation 是 inventory_reservations 表的 GORM 模型。
// order_id 是 saga 关联键（本精简 saga 中 order_id 即 saga_id）。
// UNIQUE(order_id) 保证一个订单只有一条预占记录，使补偿幂等天然成立。
type Reservation struct {
	ID        int64     `json:"id"         gorm:"primaryKey;autoIncrement"`
	OrderID   uint      `json:"order_id"   gorm:"column:order_id;uniqueIndex;not null"`
	Sku       string    `json:"sku"        gorm:"column:sku;size:64;not null"`
	Quantity  int       `json:"quantity"   gorm:"column:quantity;not null"`
	Status    string    `json:"status"     gorm:"column:status;size:32;not null;default:'reserved'"`
	CreatedAt time.Time `json:"created_at" gorm:"column:created_at;not null;default:now()"`
}

func (Reservation) TableName() string {
	return "inventory_reservations"
}

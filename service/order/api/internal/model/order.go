package model

import "gorm.io/gorm"

const (
	StatusPending   = "pending"
	StatusPaid      = "paid"
	StatusCancelled = "cancelled"
)

// Order 是 order-api 订单域的私有数据模型。
type Order struct {
	gorm.Model
	UserID       uint   `json:"user_id"       gorm:"index;not null"`
	ProductName  string `json:"product_name"  gorm:"size:256;not null"`
	Amount       int64  `json:"amount"        gorm:"not null"` // 金额，单位：分
	Status       string `json:"status"        gorm:"size:32;default:'pending'"`
	Version      int    `json:"version"       gorm:"version"` // 乐观锁版本号
	Sku          string `json:"sku"           gorm:"size:64;not null"`
	Quantity     int    `json:"quantity"      gorm:"not null;default:1"`
	CancelReason string `json:"cancel_reason" gorm:"size:32"` // OUT_OF_STOCK / USER_CANCELLED
}

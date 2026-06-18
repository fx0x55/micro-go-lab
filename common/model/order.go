package model

import "time"

const (
	StatusPending   = "pending"
	StatusPaid      = "paid"
	StatusCancelled = "cancelled"
)

type Order struct {
	ID          uint      `json:"id"           gorm:"primaryKey"`
	UserID      uint      `json:"user_id"      gorm:"index;not null"`
	ProductName string    `json:"product_name" gorm:"size:256;not null"`
	Amount      int64     `json:"amount"       gorm:"not null"` // 金额，单位：分
	Status      string    `json:"status"       gorm:"size:32;default:'pending'"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

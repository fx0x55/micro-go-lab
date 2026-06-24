package model

const (
	StatusPending   = "pending"
	StatusPaid      = "paid"
	StatusCancelled = "cancelled"
)

type Order struct {
	BaseModel
	UserID      uint   `json:"user_id"      gorm:"index;not null"`
	ProductName string `json:"product_name" gorm:"size:256;not null"`
	Amount      int64  `json:"amount"       gorm:"not null"` // 金额，单位：分
	Status      string `json:"status"       gorm:"size:32;default:'pending'"`
	Version     int    `json:"version"      gorm:"version"` // 乐观锁版本号
}

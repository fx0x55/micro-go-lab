package model

// Product 商品总量与可用量。
// available 是冗余列（= total - SUM(active reservations)），所有变更必须走事务路径。
type Product struct {
	Sku       string `json:"sku"       gorm:"column:sku;primaryKey;size:64"`
	Total     int    `json:"total"     gorm:"column:total;not null"`
	Available int    `json:"available" gorm:"column:available;not null"`
}

func (Product) TableName() string {
	return "products"
}

const (
	ReservationStatusReserved = "reserved"
	ReservationStatusReleased = "released"
)

// ReservationStatusActive 已占用的预占状态。
// released 不占用库存，不参与 available 计算。
func ReservationStatusActive(s string) bool {
	return s == ReservationStatusReserved
}

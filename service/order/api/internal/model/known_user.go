package model

import "gorm.io/gorm"

// KnownUser 是 order-api 的 CQRS 物化视图。
// 由 order-api 消费 user-events Kafka topic 填充，
// 用于在订单流程中快速判断 user 是否存在，无需每次跨服务 gRPC 校验。
type KnownUser struct {
	UserID   uint   `json:"user_id"  gorm:"column:user_id;primaryKey"`
	Username string `json:"username" gorm:"column:username;size:64;not null"`
	SeenAt   string `json:"seen_at"  gorm:"column:seen_at;type:datetime(3);not null;default:now(3)"`
}

func (KnownUser) TableName() string {
	return "known_users"
}

// UpsertKnownUser 在事务中插入或更新 known_users 行。
// 使用 GORM 的 clause.OnConflict 实现 MySQL UPSERT（INSERT ... ON DUPLICATE KEY UPDATE）。
func UpsertKnownUser(tx *gorm.DB, ku *KnownUser) error {
	return tx.Save(ku).Error
}

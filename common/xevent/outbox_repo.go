package xevent

import (
	"time"

	"gorm.io/gorm"
)

// OutboxRepository 事务性 Outbox 的 DB 操作
type OutboxRepository struct {
	db *gorm.DB
}

// NewOutboxRepository 创建 OutboxRepository
func NewOutboxRepository(db *gorm.DB) *OutboxRepository {
	return &OutboxRepository{db: db}
}

// Insert 写入 outbox 事件（需在业务事务中调用）
func (r *OutboxRepository) Insert(tx *gorm.DB, event *OutboxEvent) error {
	return tx.Create(event).Error
}

// MarkAsSent 标记事件为已发送
func (r *OutboxRepository) MarkAsSent(id int64) error {
	now := time.Now()
	return r.db.Model(&OutboxEvent{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":  OutboxStatusSent,
			"sent_at": now,
		}).Error
}

// MarkAsFailed 标记事件为失败
func (r *OutboxRepository) MarkAsFailed(id int64, errMsg string) error {
	return r.db.Model(&OutboxEvent{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":     OutboxStatusFailed,
			"last_error": errMsg,
		}).Error
}

// IncrementRetryCount 递增重试计数
func (r *OutboxRepository) IncrementRetryCount(id int64, errMsg string) error {
	return r.db.Model(&OutboxEvent{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"retry_count": gorm.Expr("retry_count + 1"),
			"last_error":  errMsg,
		}).Error
}

// FindPending 查询待发送事件
func (r *OutboxRepository) FindPending(limit int) ([]OutboxEvent, error) {
	var events []OutboxEvent
	err := r.db.
		Where("status = ? AND retry_count < ?", OutboxStatusPending, MaxRetries).
		Order("created_at ASC").
		Limit(limit).
		Find(&events).Error
	return events, err
}

package xevent

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
func (r *OutboxRepository) MarkAsSent(tx *gorm.DB, id int64) error {
	now := time.Now()
	return tx.Model(&OutboxEvent{}).
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
func (r *OutboxRepository) IncrementRetryCount(tx *gorm.DB, id int64, errMsg string) error {
	return tx.Model(&OutboxEvent{}).
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

// PendingHandler 处理一批待发送事件的回调函数。
// 在同一事务内执行，tx 可用于调用 MarkAsSent / IncrementRetryCount。
type PendingHandler func(tx *gorm.DB, events []OutboxEvent) error

// ProcessPending 在单个事务中查询待发送事件并交由 handler 处理。
//
// 使用 FOR UPDATE SKIP LOCKED：多个实例同时轮询时，每个实例拿到不同的行集合。
// 事务涵盖 SELECT + handler（含 Kafka 发布 + 状态更新），行锁在 COMMIT 后释放。
// 这保证了同一行不会被两个实例同时处理。
//
// Kafka 发布延迟通常在毫秒级，锁持有时间短，对并发吞吐影响极小。
func (r *OutboxRepository) ProcessPending(limit int, handler PendingHandler) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var events []OutboxEvent
		err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status = ? AND retry_count < ?", OutboxStatusPending, MaxRetries).
			Order("created_at ASC").
			Limit(limit).
			Find(&events).Error
		if err != nil {
			return err
		}
		if len(events) == 0 {
			return nil
		}
		return handler(tx, events)
	})
}

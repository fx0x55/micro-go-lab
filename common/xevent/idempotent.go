package xevent

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// ProcessedEvent 对应 processed_events 表
type ProcessedEvent struct {
	EventID     string    `gorm:"column:event_id;type:uuid;primaryKey"`
	ProcessedAt time.Time `gorm:"column:processed_at;autoCreateTime"`
}

// IdempotentRepository 管理事件幂等状态，供各服务消费端复用
type IdempotentRepository struct {
	db *gorm.DB
}

func NewIdempotentRepository(db *gorm.DB) *IdempotentRepository {
	return &IdempotentRepository{db: db}
}

// Process 在事务中执行幂等处理：检查事件是否已处理，若未处理则执行 fn 并标记。
// 返回 true 表示成功处理，false 表示事件已处理过（跳过）。
func (r *IdempotentRepository) Process(eventID string, fn func(tx *gorm.DB) error) (bool, error) {
	var processed bool
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var existing ProcessedEvent
		err := tx.Where("event_id = ?", eventID).First(&existing).Error
		if err == nil {
			processed = false
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		if err := fn(tx); err != nil {
			return err
		}

		if err := tx.Create(&ProcessedEvent{EventID: eventID}).Error; err != nil {
			return err
		}

		processed = true
		return nil
	})
	return processed, err
}

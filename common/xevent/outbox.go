package xevent

import "time"

const (
	OutboxStatusPending = "pending"
	OutboxStatusSent    = "sent"
	OutboxStatusFailed  = "failed"
	MaxRetries          = 5
)

// OutboxEvent 事务性 Outbox 表结构
type OutboxEvent struct {
	ID         int64     `gorm:"primaryKey;autoIncrement"`
	EventID    string    `gorm:"type:uuid;uniqueIndex;not null"`
	Topic      string    `gorm:"size:255;not null"`
	EventKey   string    `gorm:"size:255;not null"`
	EventType  string    `gorm:"size:100;not null"`
	Version    int       `gorm:"not null;default:1"`
	Payload    string    `gorm:"type:jsonb;not null"`
	Status     string    `gorm:"size:20;not null;default:'pending'"`
	RetryCount int       `gorm:"not null;default:0"`
	LastError  string    `gorm:"type:text"`
	CreatedAt  time.Time `gorm:"not null;default:now()"`
	SentAt     *time.Time
}

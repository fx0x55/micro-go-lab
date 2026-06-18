package event

import "time"

// EventType 定义事件类型
type EventType string

const (
	UserRegistered EventType = "user.registered"
)

// Event 定义事件结构
type Event struct {
	ID        uint      `json:"id"`
	Type      EventType `json:"type"`
	Payload   any       `json:"payload"`
	Status    string    `json:"status"` // "pending", "sent", "failed"
	CreatedAt time.Time `json:"created_at"`
	SentAt    time.Time `json:"sent_at,omitempty"`
}

// NewEvent 创建新事件
func NewEvent(eventType EventType, payload any) Event {
	return Event{
		Type:      eventType,
		Payload:   payload,
		Status:    "pending",
		CreatedAt: time.Now(),
	}
}

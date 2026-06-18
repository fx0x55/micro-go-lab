package event

import (
	"sync"
	"time"
)

// Outbox 定义Outbox内存存储
type Outbox struct {
	mu       sync.RWMutex
	events   []Event
	nextID   uint
	eventBus EventBus
}

// NewOutbox 创建新的Outbox
func NewOutbox(eventBus EventBus) *Outbox {
	return &Outbox{
		events:   make([]Event, 0),
		nextID:   1,
		eventBus: eventBus,
	}
}

// Add 添加事件到Outbox
func (o *Outbox) Add(event *Event) {
	o.mu.Lock()
	defer o.mu.Unlock()

	event.ID = o.nextID
	o.nextID++
	o.events = append(o.events, *event)
}

// GetPending 获取所有待处理的事件
func (o *Outbox) GetPending() []Event {
	o.mu.RLock()
	defer o.mu.RUnlock()

	var pending []Event
	for _, event := range o.events {
		if event.Status == "pending" {
			pending = append(pending, event)
		}
	}
	return pending
}

// MarkAsSent 标记事件为已发送
func (o *Outbox) MarkAsSent(id uint) {
	o.mu.Lock()
	defer o.mu.Unlock()

	for i := range o.events {
		if o.events[i].ID == id {
			o.events[i].Status = "sent"
			o.events[i].SentAt = time.Now()
			break
		}
	}
}

// MarkAsFailed 标记事件为发送失败
func (o *Outbox) MarkAsFailed(id uint) {
	o.mu.Lock()
	defer o.mu.Unlock()

	for i := range o.events {
		if o.events[i].ID == id {
			o.events[i].Status = "failed"
			break
		}
	}
}

// PublishPending 发布所有待处理的事件到EventBus
func (o *Outbox) PublishPending() int {
	pending := o.GetPending()
	published := 0

	for _, event := range pending {
		o.eventBus.Publish(&event)
		o.MarkAsSent(event.ID)
		published++
	}

	return published
}

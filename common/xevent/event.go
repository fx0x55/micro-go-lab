package xevent

import "time"

// 事件 Topic
const (
	TopicUserEvents  = "user-events"
	TopicOrderEvents = "order-events"
)

// EventType 事件类型
type EventType string

const (
	UserRegistered EventType = "user.registered"
	OrderCreated   EventType = "order.created"
)

// Envelope 是 Redis Stream 消息的统一结构
type Envelope struct {
	Event      EventType `json:"event"`
	Version    int       `json:"version"`
	OccurredAt time.Time `json:"occurred_at"`
	Data       any       `json:"data"`
}

// UserRegisteredData 用户注册事件数据
type UserRegisteredData struct {
	UserID   uint   `json:"user_id"`
	Username string `json:"username"`
}

// OrderCreatedData 订单创建事件数据
type OrderCreatedData struct {
	OrderID     uint   `json:"order_id"`
	UserID      uint   `json:"user_id"`
	ProductName string `json:"product_name"`
	Amount      int64  `json:"amount"`
	Status      string `json:"status"`
}

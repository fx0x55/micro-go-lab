package xevent

import "time"

// 事件 Topic
const (
	TopicUserEvents      = "user-events"
	TopicOrderEvents     = "order-events"
	TopicInventoryEvents = "inventory-events"
)

// TopicSpec 定义 Kafka topic 的拓扑参数（分区数、副本因子、retention）。
type TopicSpec struct {
	Name              string
	NumPartitions     int
	ReplicationFactor int
	RetentionMs       int64 // 0 表示用 broker 默认值
}

// TopicSpecs 返回本项目所有事件 topic 的拓扑定义。
// 由 Producer 启动时调用来 ensure topics 存在（IF-MISSING 语义）。
func TopicSpecs() []TopicSpec {
	return []TopicSpec{
		{
			Name:              TopicUserEvents,
			NumPartitions:     3,
			ReplicationFactor: 3,
			RetentionMs:       7 * 24 * 3600 * 1000, // 7 天
		},
		{
			Name:              TopicOrderEvents,
			NumPartitions:     3,
			ReplicationFactor: 3,
			RetentionMs:       7 * 24 * 3600 * 1000,
		},
		{
			Name:              TopicInventoryEvents,
			NumPartitions:     3,
			ReplicationFactor: 3,
			RetentionMs:       7 * 24 * 3600 * 1000, // 7 天
		},
	}
}

// EventType 事件类型
type EventType string

const (
	UserRegistered    EventType = "user.registered"
	OrderCreated      EventType = "order.created"
	OrderCancelled    EventType = "order.cancelled"
	InventoryReserved EventType = "inventory.reserved"
	ReservationFailed EventType = "inventory.reservation_failed"
)

// 取消原因
const (
	ReasonUserCancelled = "USER_CANCELLED"
	ReasonOutOfStock    = "OUT_OF_STOCK"
)

// Envelope 是 Kafka 消息的统一结构
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
	Sku         string `json:"sku"`
	Quantity    int    `json:"quantity"`
	ProductName string `json:"product_name"`
	Amount      int64  `json:"amount"`
	Status      string `json:"status"`
}

// OrderCancelledData 订单取消事件数据。
// Reason 区分用户主动取消与缺货自动取消。
type OrderCancelledData struct {
	OrderID  uint   `json:"order_id"`
	Sku      string `json:"sku"`
	Quantity int    `json:"quantity"`
	Reason   string `json:"reason"`
}

// InventoryReservedData 库存预占成功事件数据
type InventoryReservedData struct {
	OrderID uint `json:"order_id"`
}

// ReservationFailedData 库存预占失败事件数据
type ReservationFailedData struct {
	OrderID uint   `json:"order_id"`
	Reason  string `json:"reason"`
}

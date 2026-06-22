package xmetrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

const labelResult = "result"

// 业务指标：这些指标在每个服务的默认 Prometheus registry 中注册。
// 使用 prometheus.MustRegister 自动注册（init 函数执行时），因此导入此包即注册指标。
var (
	// OrdersCreated 统计订单创建情况，label: result (success, conflict, error)
	OrdersCreated = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "orders_created_total",
		Help: "Total orders created by result type.",
	}, []string{labelResult})

	// UsersRegistered 统计用户注册情况，label: result (success, exists, error)
	UsersRegistered = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "users_registered_total",
		Help: "Total users registered by result type.",
	}, []string{labelResult})

	// RPCBreakerRejected 统计熔断器拒绝的 RPC 请求，label: method
	RPCBreakerRejected = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "rpc_calls_breaker_open_total",
		Help: "Total RPC calls rejected by circuit breaker.",
	}, []string{"method"})

	// OutboxEventsPublished 统计 Outbox 事件发布情况，label: topic, result (success, error)
	OutboxEventsPublished = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "outbox_events_published_total",
		Help: "Total outbox events published by topic and result.",
	}, []string{"topic", labelResult})

	// RedisStreamMessagesConsumed 统计 Redis Stream 消息消费情况，label: topic, group, result (success, error)
	RedisStreamMessagesConsumed = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "redis_stream_messages_consumed_total",
		Help: "Total Redis Stream messages consumed by topic, group and result.",
	}, []string{"topic", "group", labelResult})
)

func init() {
	prometheus.MustRegister(
		OrdersCreated,
		UsersRegistered,
		RPCBreakerRejected,
		OutboxEventsPublished,
		RedisStreamMessagesConsumed,
	)
}

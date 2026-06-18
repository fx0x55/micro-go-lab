package xmetrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

// 业务指标：这些指标在每个服务的默认 Prometheus registry 中注册。
// 使用 prometheus.MustRegister 自动注册（init 函数执行时），因此导入此包即注册指标。
var (
	// OrdersCreated 统计订单创建情况，label: result (success, conflict, error)
	OrdersCreated = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "orders_created_total",
		Help: "Total orders created by result type.",
	}, []string{"result"})

	// UsersRegistered 统计用户注册情况，label: result (success, exists, error)
	UsersRegistered = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "users_registered_total",
		Help: "Total users registered by result type.",
	}, []string{"result"})

	// RPCBreakerRejected 统计熔断器拒绝的 RPC 请求，label: method
	RPCBreakerRejected = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "rpc_calls_breaker_open_total",
		Help: "Total RPC calls rejected by circuit breaker.",
	}, []string{"method"})
)

func init() {
	prometheus.MustRegister(OrdersCreated, UsersRegistered, RPCBreakerRejected)
}

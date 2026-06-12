package middleware

import (
	"time"

	"github.com/sony/gobreaker"
	"go.uber.org/zap"
)

func NewCircuitBreaker(name string, logger *zap.Logger) *gobreaker.CircuitBreaker {
	return gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        name,
		MaxRequests: 3,                // half-open 状态下允许的请求数
		Interval:    10 * time.Second, // closed 状态下的统计周期
		Timeout:     5 * time.Second,  // open 状态持续时间
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			// 连续失败 5 次触发熔断
			return counts.ConsecutiveFailures >= 5
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			logger.Warn("circuit breaker state changed",
				zap.String("name", name),
				zap.String("from", from.String()),
				zap.String("to", to.String()),
			)
		},
	})
}

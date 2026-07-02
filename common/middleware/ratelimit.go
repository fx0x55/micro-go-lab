package middleware

import (
	"net/http"
	"time"

	"github.com/zeromicro/go-zero/core/limit"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/stores/redis"
)

// RedisRateLimiter 基于 go-zero PeriodLimit 的分布式 IP 限流中间件。
// 使用 Redis 存储计数，支持多实例部署。
type RedisRateLimiter struct {
	periodLimit *limit.PeriodLimit
	window      time.Duration
}

// NewRedisRateLimiter 创建 Redis 限流器。
// rate 为每个 window 时间窗口内允许的最大请求数；keyPrefix 用于 Redis key 前缀（如 "ratelimit:user-api:"）。
func NewRedisRateLimiter(rds *redis.Redis, rate int, window time.Duration, keyPrefix string) *RedisRateLimiter {
	period := max(int(window.Seconds()), 1)

	return &RedisRateLimiter{
		periodLimit: limit.NewPeriodLimit(period, rate, rds, keyPrefix),
		window:      window,
	}
}

// Middleware 返回 go-zero 兼容的限流中间件。
func (rl *RedisRateLimiter) Middleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)

		code, err := rl.periodLimit.TakeCtx(r.Context(), ip)
		if err != nil {
			logx.WithContext(r.Context()).Error("rate limiter error",
				logx.Field("ip", ip), logx.Field("error", err))
			next(w, r)
			return
		}

		if code == limit.OverQuota {
			ErrorJson(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}

		next(w, r)
	}
}

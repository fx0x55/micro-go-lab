package middleware

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// RateLimiter 基于滑动窗口的 IP 限流中间件。
type RateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	rate     int           // 每窗口允许的请求数
	window   time.Duration // 窗口大小
}

type visitor struct {
	count    int
	lastSeen time.Time
}

// NewRateLimiter 创建限流器，rate 为每个 window 时间窗口内允许的最大请求数。
func NewRateLimiter(rate int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		visitors: make(map[string]*visitor),
		rate:     rate,
		window:   window,
	}
	go rl.cleanup()
	return rl
}

func (rl *RateLimiter) cleanup() {
	for {
		time.Sleep(rl.window * 2)
		rl.mu.Lock()
		for ip, v := range rl.visitors {
			if time.Since(v.lastSeen) > rl.window*2 {
				delete(rl.visitors, ip)
			}
		}
		rl.mu.Unlock()
	}
}

func (rl *RateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	v, exists := rl.visitors[ip]
	now := time.Now()

	if !exists || now.Sub(v.lastSeen) > rl.window {
		rl.visitors[ip] = &visitor{count: 1, lastSeen: now}
		return true
	}

	if v.count >= rl.rate {
		return false
	}

	v.count++
	v.lastSeen = now
	return true
}

// Middleware 返回 go-zero 兼容的限流中间件。
func (rl *RateLimiter) Middleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)

		if !rl.allow(ip) {
			ErrorJson(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}

		next(w, r)
	}
}

// clientIP 提取客户端 IP。
//
// 注意生产环境的两点限制：
//  1. X-Forwarded-For 可被客户端伪造。只有在请求经由可信代理/网关到达、且网关
//     会覆盖该头时，才能信任其中的值。生产建议信任网关注入的 X-Real-IP，
//     并对直连客户端的 X-Forwarded-For 保持警惕。
//  2. 本限流器是进程内计数（map+mutex），仅在单实例下有效。多副本部署时各算各的，
//     需改用 Redis 版（go-zero core/limit tokenlimiter/periodlimit）。
func clientIP(r *http.Request) string {
	// X-Forwarded-For: client, proxy1, proxy2 —— 取最左（第一个）并 trim。
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			xff = xff[:i]
		}
		if ip := strings.TrimSpace(xff); ip != "" {
			return ip
		}
	}
	// 回退到 RemoteAddr，去掉端口。
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

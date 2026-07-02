package middleware

import (
	"net"
	"net/http"
	"strings"
)

// clientIP 提取客户端 IP。
//
// 注意生产环境的两点限制：
//  1. X-Forwarded-For 可被客户端伪造。只有在请求经由可信代理/网关到达、且网关
//     会覆盖该头时，才能信任其中的值。生产建议信任网关注入的 X-Real-IP，
//     并对直连客户端的 X-Forwarded-For 保持警惕。
//  2. 本限流器的 key 基于 IP，多副本部署时各算各的，需改用 Redis 版（go-zero core/limit）。
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

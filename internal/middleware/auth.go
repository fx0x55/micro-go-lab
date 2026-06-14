package middleware

import "net/http"

// GetUserID 从请求上下文提取 JWT 中的 user_id。
// go-zero 的 JWT 中间件把数字 claim 存为 float64。
func GetUserID(r *http.Request) uint {
	v := r.Context().Value("user_id")
	if v == nil {
		return 0
	}
	if f, ok := v.(float64); ok {
		return uint(f)
	}
	if id, ok := v.(uint); ok {
		return id
	}
	return 0
}

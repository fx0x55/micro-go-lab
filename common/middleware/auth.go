package middleware

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// GetUserID 从请求上下文提取 JWT 中的 user_id。
// go-zero 的 JWT parser 使用 WithJSONNumber()，数字 claim 存为 json.Number。
func GetUserID(r *http.Request) uint {
	v := r.Context().Value("user_id")
	if v == nil {
		return 0
	}
	switch n := v.(type) {
	case json.Number:
		id, err := n.Int64()
		if err != nil || id < 0 {
			return 0
		}
		return uint(id)
	case float64:
		if n < 0 {
			return 0
		}
		return uint(n)
	case uint:
		return n
	case int:
		return uint(n)
	case string:
		id, err := strconv.ParseUint(n, 10, 64)
		if err != nil {
			return 0
		}
		return uint(id)
	default:
		return 0
	}
}

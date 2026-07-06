package middleware

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// ContextKey 用于 context.Value 的自定义 key 类型，避免与其它包的 string key 碰撞。
type ContextKey string

// CtxKeyUserID 是 user_id 在 context 中的 key。
// GetUserID 优先读此 key；若未找到，回退读 go-zero JWT 中间件写入的 string key "user_id"。
const CtxKeyUserID ContextKey = "user_id"

// goZeroUserIDKey 是 go-zero JWT 中间件使用的 context key（内置 string 类型）。
const goZeroUserIDKey = "user_id"

// GetUserID 从请求上下文提取 JWT 中的 user_id。
// 优先读 CtxKeyUserID（类型安全），回退读 go-zero JWT string key。
func GetUserID(r *http.Request) uint {
	v := r.Context().Value(CtxKeyUserID)
	if v == nil {
		v = r.Context().Value(goZeroUserIDKey)
	}
	if v == nil {
		return 0
	}
	return parseUserID(v)
}

func parseUserID(v any) uint {
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

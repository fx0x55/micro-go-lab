package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
)

type ContextKey string

const CtxKeyUserID ContextKey = "user_id"

const goZeroUserIDKey = "user_id"

func GetUserID(r *http.Request) uint {
	return GetUserIDFromContext(r.Context())
}

func GetUserIDFromContext(ctx context.Context) uint {
	v := ctx.Value(CtxKeyUserID)
	if v == nil {
		v = ctx.Value(goZeroUserIDKey)
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

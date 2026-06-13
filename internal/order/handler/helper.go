package handler

import (
	"net/http"
)

func getUserID(r *http.Request) uint {
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

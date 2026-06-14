package middleware

import (
	"encoding/json"
	"net/http"
)

// HealthHandler 返回服务健康状态
func HealthHandler(serviceName string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "service": serviceName})
	}
}

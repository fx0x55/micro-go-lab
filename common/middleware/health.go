package middleware

import (
	"encoding/json"
	"net/http"
)

// HealthHandler 返回服务健康状态。传入的 checks 任一失败则返回 503。
func HealthHandler(serviceName string, checks ...func() error) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		for _, check := range checks {
			if err := check(); err != nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"status":  "unhealthy",
					"service": serviceName,
					"error":   err.Error(),
				})
				return
			}
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "service": serviceName})
	}
}

package middleware

import (
	"net/http"
	"slices"

	"github.com/fx0x55/micro-go-lab/common/config"
)

// NewCorsMiddleware 返回根据 CORSConfig 配置的 CORS 中间件。
// AllowedOrigins 为 ["*"] 时允许所有来源；否则仅允许白名单中的来源。
func NewCorsMiddleware(cfg config.CORSConfig) func(http.HandlerFunc) http.HandlerFunc {
	allowAll := len(cfg.AllowedOrigins) == 1 && cfg.AllowedOrigins[0] == "*"
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if allowAll {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			} else if origin != "" && isAllowedOrigin(origin, cfg.AllowedOrigins) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
			}

			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Requested-With")
			w.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Type")
			w.Header().Set("Access-Control-Max-Age", "86400")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next(w, r)
		}
	}
}

// CorsMiddleware 是默认的 CORS 中间件（允许所有来源）。
// 保留此函数以兼容未迁移的调用点。
func CorsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return NewCorsMiddleware(config.CORSConfig{AllowedOrigins: []string{"*"}})(next)
}

func isAllowedOrigin(origin string, allowed []string) bool {
	return slices.Contains(allowed, origin)
}

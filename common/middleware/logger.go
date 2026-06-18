package middleware

import (
	"bytes"
	"io"
	"net/http"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
)

// responseWriter 包装 http.ResponseWriter 以捕获状态码。
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	buf        bytes.Buffer
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	rw.buf.Write(b)
	return rw.ResponseWriter.Write(b)
}

// RequestLogger 记录每个请求的方法、路径、状态码、耗时、客户端IP。
func RequestLogger(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// 读取请求体用于日志（可选，限制大小避免内存问题）
		var bodyBytes []byte
		if r.Body != nil && r.ContentLength > 0 && r.ContentLength < 10240 {
			bodyBytes, _ = io.ReadAll(r.Body)
			r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}

		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next(rw, r)

		duration := time.Since(start)

		clientIP := r.Header.Get("X-Forwarded-For")
		if clientIP == "" {
			clientIP = r.RemoteAddr
		}

		logx.WithContext(r.Context()).Infow("HTTP request",
			logx.Field("method", r.Method),
			logx.Field("path", r.URL.Path),
			logx.Field("status", rw.statusCode),
			logx.Field("duration", duration.String()),
			logx.Field("ip", clientIP),
		)
	}
}

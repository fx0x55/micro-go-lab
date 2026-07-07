package client

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/fx0x55/micro-go-lab/common/xmetrics"
	"github.com/zeromicro/go-zero/core/breaker"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	maxRetryAttempts = 3
	baseBackoff      = 100 * time.Millisecond
)

// RetryUnaryInterceptor 对临时性错误按指数退避重试。
// Q6：retry 从厚封装迁出，统一注入 svc 的 zrpc.MustNewClient。
func RetryUnaryInterceptor(
	ctx context.Context,
	method string,
	req, reply any,
	cc *grpc.ClientConn,
	invoker grpc.UnaryInvoker,
	opts ...grpc.CallOption,
) error {
	var lastErr error
	for i := range maxRetryAttempts {
		err := invoker(ctx, method, req, reply, cc, opts...)
		if err == nil {
			return nil
		}
		lastErr = err

		// breaker rejection 指标埋点（Q4：从厚封装迁出，按方法名统一覆盖）
		if errors.Is(err, breaker.ErrServiceUnavailable) {
			xmetrics.RPCBreakerRejected.WithLabelValues(shortMethod(method)).Inc()
			logx.Error("gRPC breaker rejected", logx.Field("method", method), logx.Field("error", err))
		}

		if !isRetryable(err) || i == maxRetryAttempts-1 {
			return err
		}
		backoff := baseBackoff << i
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return lastErr
}

// shortMethod 从完整 gRPC method（/pkg.Service/Method）提取方法名作为 label。
func shortMethod(method string) string {
	if idx := strings.LastIndex(method, "/"); idx >= 0 {
		return method[idx+1:]
	}
	return method
}

func isRetryable(err error) bool {
	st, ok := status.FromError(err)
	if !ok {
		return false
	}
	switch st.Code() {
	case codes.Unavailable, codes.DeadlineExceeded, codes.ResourceExhausted:
		return true
	}
	return false
}

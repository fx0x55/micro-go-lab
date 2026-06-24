package xstream

import (
	"context"
	"runtime/debug"

	"github.com/zeromicro/go-zero/core/logx"
)

// RunWithRecover runs fn in a goroutine-safe way with panic recovery.
// If fn panics, the panic is logged (with stack trace) instead of crashing the process.
// The log fields identify which goroutine crashed (caller supplies "caller").
func RunWithRecover(ctx context.Context, caller string, fn func(ctx context.Context)) {
	defer func() {
		if r := recover(); r != nil {
			logx.WithContext(ctx).Error("goroutine panic recovered",
				logx.Field("caller", caller),
				logx.Field("panic", r),
				logx.Field("stack", string(debug.Stack())),
			)
		}
	}()
	fn(ctx)
}

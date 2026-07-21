package xdb

import (
	"context"
	"errors"
	"math/rand/v2"
	"time"

	"github.com/fx0x55/micro-go-lab/common/xmetrics"
	"github.com/go-sql-driver/mysql"
)

// MySQL 可安全重试的锁相关错误码。
const (
	errDeadlock        = 1213 // ER_LOCK_DEADLOCK：InnoDB 主动检测到环并回滚了 victim 事务
	errLockWaitTimeout = 1205 // ER_LOCK_WAIT_TIMEOUT：锁等待超过 innodb_lock_wait_timeout（默认 50s）
)

// RetryOptions 控制事务重试行为。
type RetryOptions struct {
	MaxAttempts int           // 含首次在内的总尝试次数
	BaseDelay   time.Duration // 指数退避基数
	MaxDelay    time.Duration // 退避上限
}

// DefaultRetryOptions 是死锁重试的默认参数：最多 3 次，50ms 起退避，封顶 500ms。
func DefaultRetryOptions() RetryOptions {
	return RetryOptions{
		MaxAttempts: 3,
		BaseDelay:   50 * time.Millisecond,
		MaxDelay:    500 * time.Millisecond,
	}
}

// IsDeadlock 判断 err 是否为可安全重试的锁相关错误（1213 死锁 / 1205 锁等待超时）。
// 用 errors.As 而非类型断言，因此 err 即使被 fmt.Errorf("...: %w") 或 gorm 包装也能识别。
func IsDeadlock(err error) bool {
	var myErr *mysql.MySQLError
	if errors.As(err, &myErr) {
		switch myErr.Number {
		case errDeadlock, errLockWaitTimeout:
			return true
		}
	}
	return false
}

// WithRetry 在可重试的锁错误（1213/1205）上重试 fn。
//
// 关键约束：fn 每次执行都必须自己开一个全新事务（典型写法是在 fn 内部调用
// db.Transaction(func(tx *gorm.DB) error { ... })）。死锁/锁等待会把当前事务整体回滚，
// 复用已回滚的 tx 会直接报错；因此 fn 不接收 *gorm.DB 参数——从类型层面就杜绝了
// "在同一个废掉的 tx 上重试"的写法。
//
// db 用于给 db_deadlocks_total 指标打标（数据库名）。上下文取消会中断退避等待并返回 ctx.Err()。
func WithRetry(ctx context.Context, db string, fn func() error, opts ...RetryOptions) error {
	o := DefaultRetryOptions()
	if len(opts) > 0 {
		o = opts[0]
	}

	var err error
	for attempt := 1; attempt <= o.MaxAttempts; attempt++ {
		err = fn()
		if err == nil {
			return nil
		}
		if !IsDeadlock(err) {
			return err // 非锁错误：立即返回，不重试
		}

		// 捕获到锁错误：最后一次记为 exhausted，否则记为 retried 并退避后重试。
		if attempt >= o.MaxAttempts {
			xmetrics.DBDeadlocks.WithLabelValues(db, "exhausted").Inc()
			return err
		}
		xmetrics.DBDeadlocks.WithLabelValues(db, "retried").Inc()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff(o.BaseDelay, o.MaxDelay, attempt)):
		}
	}
	return err
}

// backoff 返回指数退避 + 抖动后的等待时长：base*2^(attempt-1) 封顶 maxDelay，再减去 [0, d/2) 的抖动。
func backoff(base, maxDelay time.Duration, attempt int) time.Duration {
	d := base
	for range attempt - 1 {
		d <<= 1
		if d > maxDelay {
			d = maxDelay
			break
		}
	}
	if jitter := d / 2; jitter > 0 {
		// 退避抖动用非加密随机是有意为之（gosec G404 在此不适用）。
		d -= time.Duration(rand.Int64N(int64(jitter))) //nolint:gosec // 退避抖动，非安全场景
	}
	return d
}

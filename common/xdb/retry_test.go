package xdb

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
)

func fakeDeadlock() error { return &mysql.MySQLError{Number: errDeadlock, Message: "Deadlock"} }
func fakeLockWait() error { return &mysql.MySQLError{Number: errLockWaitTimeout, Message: "Lock wait"} }
func fakeDup() error      { return &mysql.MySQLError{Number: 1062, Message: "Duplicate"} }

func fastOpts() RetryOptions {
	return RetryOptions{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond}
}

func TestWithRetry_SucceedsFirstTry(t *testing.T) {
	calls := 0
	err := WithRetry(context.Background(), "testdb", func() error {
		calls++
		return nil
	}, fastOpts())
	if err != nil {
		t.Fatalf("want nil, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("want 1 call, got %d", calls)
	}
}

func TestWithRetry_RetriesOnDeadlockThenSucceeds(t *testing.T) {
	calls := 0
	err := WithRetry(context.Background(), "testdb", func() error {
		calls++
		if calls < 2 {
			return fakeDeadlock()
		}
		return nil
	}, fastOpts())
	if err != nil {
		t.Fatalf("want nil, got %v", err)
	}
	if calls != 2 {
		t.Fatalf("want 2 calls, got %d", calls)
	}
}

func TestWithRetry_ExhaustsAttemptsAndReturnsDeadlock(t *testing.T) {
	calls := 0
	de := fakeDeadlock()
	err := WithRetry(context.Background(), "testdb", func() error {
		calls++
		return de
	}, fastOpts())
	if !errors.Is(err, de) {
		t.Fatalf("want the deadlock error, got %v", err)
	}
	if calls != 3 {
		t.Fatalf("want 3 calls (== MaxAttempts), got %d", calls)
	}
}

func TestWithRetry_NonRetryableReturnedImmediately(t *testing.T) {
	calls := 0
	dup := fakeDup()
	err := WithRetry(context.Background(), "testdb", func() error {
		calls++
		return dup
	}, fastOpts())
	if !errors.Is(err, dup) {
		t.Fatalf("want the dup error, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("want 1 call (no retry on 1062), got %d", calls)
	}
}

func TestWithRetry_ContextCancelAbortsBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()
	err := WithRetry(ctx, "testdb", fakeDeadlock,
		RetryOptions{MaxAttempts: 5, BaseDelay: 100 * time.Millisecond, MaxDelay: 200 * time.Millisecond})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}

func TestIsDeadlock(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"deadlock", fakeDeadlock(), true},
		{"lock-wait", fakeLockWait(), true},
		{"wrapped-deadlock", fmt.Errorf("tx failed: %w", fakeDeadlock()), true},
		{"duplicate", fakeDup(), false},
		{"generic", errors.New("boom"), false},
		{"nil", nil, false},
	}
	for _, c := range cases {
		if got := IsDeadlock(c.err); got != c.want {
			t.Errorf("IsDeadlock(%s)=%v want %v", c.name, got, c.want)
		}
	}
}

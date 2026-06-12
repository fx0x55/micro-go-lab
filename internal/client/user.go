package client

import (
	"context"
	"errors"
	"time"

	userv1 "github.com/wokoworks/go-server/gen/user/v1"
	"github.com/sony/gobreaker"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/wokoworks/go-server/internal/middleware"
)

var ErrCircuitOpen = errors.New("user service circuit breaker is open")

type UserClient struct {
	conn      *grpc.ClientConn
	client    userv1.UserServiceClient
	logger    *zap.Logger
	cb        *gobreaker.CircuitBreaker
	maxRetry  int
	baseDelay time.Duration
}

func NewUserClient(addr string, logger *zap.Logger) (*UserClient, error) {
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	)
	if err != nil {
		return nil, err
	}
	return &UserClient{
		conn:      conn,
		client:    userv1.NewUserServiceClient(conn),
		logger:    logger,
		cb:        middleware.NewCircuitBreaker("user-svc", logger),
		maxRetry:  3,
		baseDelay: 100 * time.Millisecond,
	}, nil
}

func (c *UserClient) ValidateUser(ctx context.Context, userID uint) (bool, error) {
	var result bool
	err := c.retryWithBackoff(func() error {
		resp, err := c.cb.Execute(func() (interface{}, error) {
			return c.client.ValidateUser(ctx, &userv1.ValidateUserRequest{
				UserId: uint64(userID),
			})
		})
		if err != nil {
			if errors.Is(err, gobreaker.ErrOpenState) {
				return ErrCircuitOpen
			}
			return err
		}
		result = resp.(*userv1.ValidateUserResponse).Exists
		return nil
	})
	if err != nil {
		c.logger.Error("gRPC ValidateUser failed after retries", zap.Error(err))
		return false, err
	}
	return result, nil
}

func (c *UserClient) GetUser(ctx context.Context, userID uint) (*userv1.GetUserResponse, error) {
	var result *userv1.GetUserResponse
	err := c.retryWithBackoff(func() error {
		resp, err := c.cb.Execute(func() (interface{}, error) {
			return c.client.GetUser(ctx, &userv1.GetUserRequest{
				UserId: uint64(userID),
			})
		})
		if err != nil {
			if errors.Is(err, gobreaker.ErrOpenState) {
				return ErrCircuitOpen
			}
			return err
		}
		result = resp.(*userv1.GetUserResponse)
		return nil
	})
	if err != nil {
		c.logger.Error("gRPC GetUser failed after retries", zap.Error(err))
		return nil, err
	}
	return result, nil
}

func (c *UserClient) retryWithBackoff(fn func() error) error {
	var lastErr error
	for i := 0; i <= c.maxRetry; i++ {
		lastErr = fn()
		if lastErr == nil {
			return nil
		}
		// 熔断器打开时不重试
		if errors.Is(lastErr, ErrCircuitOpen) {
			return lastErr
		}
		if i < c.maxRetry {
			delay := c.baseDelay * time.Duration(1<<uint(i))
			c.logger.Warn("retrying gRPC call",
				zap.Int("attempt", i+1),
				zap.Duration("delay", delay),
				zap.Error(lastErr),
			)
			time.Sleep(delay)
		}
	}
	return lastErr
}

func (c *UserClient) Close() error {
	return c.conn.Close()
}

package client

import (
	"context"
	"errors"

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
	conn   *grpc.ClientConn
	client userv1.UserServiceClient
	logger *zap.Logger
	cb     *gobreaker.CircuitBreaker
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
		conn:   conn,
		client: userv1.NewUserServiceClient(conn),
		logger: logger,
		cb:     middleware.NewCircuitBreaker("user-svc", logger),
	}, nil
}

func (c *UserClient) ValidateUser(ctx context.Context, userID uint) (bool, error) {
	result, err := c.cb.Execute(func() (interface{}, error) {
		return c.client.ValidateUser(ctx, &userv1.ValidateUserRequest{
			UserId: uint64(userID),
		})
	})
	if err != nil {
		if errors.Is(err, gobreaker.ErrOpenState) {
			c.logger.Warn("circuit breaker open, fast-failing ValidateUser")
			return false, ErrCircuitOpen
		}
		c.logger.Error("gRPC ValidateUser failed", zap.Error(err))
		return false, err
	}
	return result.(*userv1.ValidateUserResponse).Exists, nil
}

func (c *UserClient) GetUser(ctx context.Context, userID uint) (*userv1.GetUserResponse, error) {
	result, err := c.cb.Execute(func() (interface{}, error) {
		return c.client.GetUser(ctx, &userv1.GetUserRequest{
			UserId: uint64(userID),
		})
	})
	if err != nil {
		if errors.Is(err, gobreaker.ErrOpenState) {
			c.logger.Warn("circuit breaker open, fast-failing GetUser")
			return nil, ErrCircuitOpen
		}
		c.logger.Error("gRPC GetUser failed", zap.Error(err))
		return nil, err
	}
	return result.(*userv1.GetUserResponse), nil
}

func (c *UserClient) Close() error {
	return c.conn.Close()
}

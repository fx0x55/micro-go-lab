package client

import (
	"context"

	userv1 "github.com/wokoworks/go-server/gen/user/v1"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type UserClient struct {
	conn   *grpc.ClientConn
	client userv1.UserServiceClient
	logger *zap.Logger
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
	}, nil
}

func (c *UserClient) ValidateUser(ctx context.Context, userID uint) (bool, error) {
	resp, err := c.client.ValidateUser(ctx, &userv1.ValidateUserRequest{
		UserId: uint64(userID),
	})
	if err != nil {
		c.logger.Error("gRPC ValidateUser failed", zap.Error(err))
		return false, err
	}
	return resp.Exists, nil
}

func (c *UserClient) GetUser(ctx context.Context, userID uint) (*userv1.GetUserResponse, error) {
	resp, err := c.client.GetUser(ctx, &userv1.GetUserRequest{
		UserId: uint64(userID),
	})
	if err != nil {
		c.logger.Error("gRPC GetUser failed", zap.Error(err))
		return nil, err
	}
	return resp, nil
}

func (c *UserClient) Close() error {
	return c.conn.Close()
}

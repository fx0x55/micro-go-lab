package client

import (
	"context"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"

	userv1 "github.com/wokoworks/go-server/gen/user/v1"
	"github.com/wokoworks/go-server/internal/config"
)

// UserSummary 是 ValidateUser 返回的用户摘要，包含存在性与用户名
type UserSummary struct {
	Exists   bool
	Username string
}

type UserClient struct {
	client userv1.UserServiceClient
	conn   *grpc.ClientConn
}

func NewUserClient(cfg config.UserSvcConfig) *UserClient {
	cli := zrpc.MustNewClient(zrpc.RpcClientConf{
		Etcd:      cfg.Etcd,
		Endpoints: cfg.Endpoints,
		Timeout:   cfg.Timeout,
		NonBlock:  true,
	})
	return &UserClient{
		client: userv1.NewUserServiceClient(cli.Conn()),
		conn:   cli.Conn(),
	}
}

func (c *UserClient) ValidateUser(ctx context.Context, userID uint) (*UserSummary, error) {
	resp, err := c.client.ValidateUser(ctx, &userv1.ValidateUserRequest{
		UserId: uint64(userID),
	})
	if err != nil {
		logx.Error("gRPC ValidateUser failed", logx.Field("error", err))
		return nil, err
	}
	return &UserSummary{Exists: resp.Exists, Username: resp.Username}, nil
}

func (c *UserClient) GetUser(ctx context.Context, userID uint) (*userv1.GetUserResponse, error) {
	resp, err := c.client.GetUser(ctx, &userv1.GetUserRequest{
		UserId: uint64(userID),
	})
	if err != nil {
		logx.Error("gRPC GetUser failed", logx.Field("error", err))
		return nil, err
	}
	return resp, nil
}

func (c *UserClient) Close() error {
	return c.conn.Close()
}

package client

import (
	"context"
	"errors"

	"github.com/zeromicro/go-zero/core/breaker"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"

	"github.com/wokoworks/go-server/common/config"
	"github.com/wokoworks/go-server/common/xmetrics"
	userv1 "github.com/wokoworks/go-server/service/user/rpc/pb"
)

type UserSummary struct {
	Exists   bool
	Username string
}

type UserClient struct {
	client userv1.UserServiceClient
	conn   *grpc.ClientConn
}

// buildRpcClientConf 把 UserSvcConfig 转成 go-zero 的 RpcClientConf。
//
// 关键：直接构造 RpcClientConf 字面量不会走 conf 解析，Middlewares 上的
// `json:",default=true"` 标签不生效，会被当成零值 false，进而导致
// Trace/Breaker/Timeout 等客户端拦截器被 buildUnaryInterceptors 跳过
// （见 go-zero internal/client.go 的 if c.middlewares.Trace），
// order-api → user-rpc 的链路追踪、熔断与超时会随之失效。
// 不能用 conf.FillDefault 补默认值——它要求整个结构体为空（Etcd/Timeout 已赋值时会 panic），
// 因此这里显式把 Middlewares 各项置 true，与 RpcServerConf 经 conf.MustLoad 后的默认行为一致。
func buildRpcClientConf(cfg config.UserSvcConfig) zrpc.RpcClientConf {
	return zrpc.RpcClientConf{
		Etcd:      cfg.Etcd,
		Endpoints: cfg.Endpoints,
		Timeout:   cfg.Timeout,
		NonBlock:  true,
		Middlewares: zrpc.ClientMiddlewaresConf{
			Trace:      true,
			Duration:   true,
			Prometheus: true,
			Breaker:    true,
			Timeout:    true,
		},
	}
}

func NewUserClient(cfg config.UserSvcConfig) *UserClient {
	cli := zrpc.MustNewClient(buildRpcClientConf(cfg), zrpc.WithUnaryClientInterceptor(retryUnaryInterceptor))
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
		if errors.Is(err, breaker.ErrServiceUnavailable) {
			xmetrics.RPCBreakerRejected.WithLabelValues("ValidateUser").Inc()
		}
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
		if errors.Is(err, breaker.ErrServiceUnavailable) {
			xmetrics.RPCBreakerRejected.WithLabelValues("GetUser").Inc()
		}
		logx.Error("gRPC GetUser failed", logx.Field("error", err))
		return nil, err
	}
	return resp, nil
}

func (c *UserClient) Close() error {
	return c.conn.Close()
}

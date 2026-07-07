package svc

import (
	"fmt"
	"time"

	"github.com/fx0x55/micro-go-lab/common/client"
	"github.com/fx0x55/micro-go-lab/common/middleware"
	"github.com/fx0x55/micro-go-lab/service/user/api/internal/config"
	"github.com/fx0x55/micro-go-lab/service/user/rpc/userservice"
	gozeroRedis "github.com/zeromicro/go-zero/core/stores/redis"
	"github.com/zeromicro/go-zero/zrpc"
)

// user-api 是纯网关：不连库、不写 outbox、不消费事件。
// 仅持有 user-rpc 的 gRPC 客户端 + Redis（限流后端）+ JWT 配置。
type ServiceContext struct {
	Config      config.Config
	UserCli     userservice.UserService
	rpcCli      zrpc.Client
	RateLimiter *middleware.RedisRateLimiter
}

func NewServiceContext(c *config.Config) *ServiceContext {
	// retry 拦截器统一注入（Q6：svc 不被 goctl 覆盖，retry 手写在此）
	rpcCli := zrpc.MustNewClient(c.UserSvc.RpcClientConf(),
		zrpc.WithUnaryClientInterceptor(client.RetryUnaryInterceptor))

	rdb := gozeroRedis.New(c.Redis.Addr(), gozeroRedis.WithPass(c.Redis.Password))
	rateLimiter := middleware.NewRedisRateLimiter(rdb, 1, time.Second*3, "ratelimit:user-api:")

	return &ServiceContext{
		Config:      *c,
		UserCli:     userservice.NewUserService(rpcCli),
		rpcCli:      rpcCli,
		RateLimiter: rateLimiter,
	}
}

func (sc *ServiceContext) Stop() {
	if sc.rpcCli != nil {
		if err := sc.rpcCli.Conn().Close(); err != nil {
			fmt.Printf("close user-rpc client: %v\n", err)
		}
	}
}

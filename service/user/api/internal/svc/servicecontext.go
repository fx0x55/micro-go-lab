package svc

import (
	"fmt"
	"time"

	"github.com/fx0x55/micro-go-lab/common/client"
	"github.com/fx0x55/micro-go-lab/common/middleware"
	"github.com/fx0x55/micro-go-lab/service/user/api/internal/config"
	gozeroRedis "github.com/zeromicro/go-zero/core/stores/redis"
)

// user-api 是纯网关：不连库、不写 outbox、不消费事件。
// 仅持有 user-rpc 的 gRPC 客户端 + Redis（限流后端）+ JWT 配置。
type ServiceContext struct {
	Config      config.Config
	UserCli     *client.UserClient
	RateLimiter *middleware.RedisRateLimiter
}

func NewServiceContext(c *config.Config) *ServiceContext {
	userCli := client.NewUserClient(&c.UserSvc)

	rdb := gozeroRedis.New(c.Redis.Addr(), gozeroRedis.WithPass(c.Redis.Password))
	rateLimiter := middleware.NewRedisRateLimiter(rdb, 1, time.Second*3, "ratelimit:user-api:")

	return &ServiceContext{
		Config:      *c,
		UserCli:     userCli,
		RateLimiter: rateLimiter,
	}
}

// Stop 网关无后台 goroutine，保留方法签名供 main 统一调用（未来加连接池关闭时用）。
func (sc *ServiceContext) Stop() {
	if err := sc.UserCli.Close(); err != nil {
		fmt.Printf("close user-rpc client: %v\n", err)
	}
}

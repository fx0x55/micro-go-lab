package svc

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/fx0x55/micro-go-lab/common/client"
	"github.com/fx0x55/micro-go-lab/common/middleware"
	"github.com/fx0x55/micro-go-lab/common/xdb"
	"github.com/fx0x55/micro-go-lab/common/xevent"
	"github.com/fx0x55/micro-go-lab/common/xredis"
	"github.com/fx0x55/micro-go-lab/common/xstream"
	"github.com/fx0x55/micro-go-lab/service/order/api/internal/config"
	"github.com/fx0x55/micro-go-lab/service/order/api/internal/repository"
	"github.com/fx0x55/micro-go-lab/service/user/rpc/userservice"
	"github.com/redis/go-redis/v9"
	"github.com/zeromicro/go-zero/core/logx"
	gozeroRedis "github.com/zeromicro/go-zero/core/stores/redis"
	"github.com/zeromicro/go-zero/zrpc"
	"gorm.io/gorm"
)

type ServiceContext struct {
	Config      config.Config
	DB          *gorm.DB
	OrderRepo   repository.OrderRepositoryInterface
	UserCli     userservice.UserService
	RpcCli      zrpc.Client
	Redis       *redis.Client
	OutboxRepo  *xevent.OutboxRepository
	Producer    *xstream.Producer
	Poller      *xstream.Poller
	RateLimiter *middleware.RedisRateLimiter
	cancel      context.CancelFunc
	wg          sync.WaitGroup
}

func NewServiceContext(ctx context.Context, c *config.Config) *ServiceContext {
	gormDB, err := xdb.New(&c.Database)
	if err != nil {
		panic(fmt.Sprintf("failed to connect database: %v", err))
	}
	if err := xdb.Migrate(ctx, gormDB, "order"); err != nil {
		panic(fmt.Sprintf("failed to migrate: %v", err))
	}

	// retry 拦截器统一注入（Q6：svc 不被 goctl 覆盖，retry 手写在此）
	rpcCli := zrpc.MustNewClient(c.UserSvc.RpcClientConf(),
		zrpc.WithUnaryClientInterceptor(client.RetryUnaryInterceptor))

	redisClient, err := xredis.New(ctx, c.Redis)
	if err != nil {
		panic(fmt.Sprintf("failed to connect redis: %v", err))
	}

	ctx, cancel := context.WithCancel(ctx)

	outboxRepo := xevent.NewOutboxRepository(gormDB)
	producer := xstream.NewProducer(redisClient)
	poller := xstream.NewPoller(outboxRepo, producer, 5*time.Second, 100)

	rateLimiter := middleware.NewRedisRateLimiter(
		gozeroRedis.New(c.Redis.Addr(), gozeroRedis.WithPass(c.Redis.Password)),
		100, time.Minute, "ratelimit:order-api:",
	)

	sc := &ServiceContext{
		Config:      *c,
		DB:          gormDB,
		OrderRepo:   repository.NewOrderRepository(gormDB),
		UserCli:     userservice.NewUserService(rpcCli),
		RpcCli:      rpcCli,
		Redis:       redisClient,
		OutboxRepo:  outboxRepo,
		Producer:    producer,
		Poller:      poller,
		RateLimiter: rateLimiter,
		cancel:      cancel,
	}

	poller.Start(ctx, &sc.wg)

	return sc
}

func (sc *ServiceContext) Stop() {
	sc.cancel()
	sc.wg.Wait()
	logx.Info("all background goroutines stopped")
}

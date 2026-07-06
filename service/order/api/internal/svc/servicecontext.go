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
	"github.com/redis/go-redis/v9"
	"github.com/zeromicro/go-zero/core/logx"
	gozeroRedis "github.com/zeromicro/go-zero/core/stores/redis"
	"gorm.io/gorm"
)

type ServiceContext struct {
	Config      config.Config
	DB          *gorm.DB
	OrderRepo   repository.OrderRepositoryInterface
	UserCli     *client.UserClient
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

	userCli := client.NewUserClient(&c.UserSvc)

	redisClient, err := xredis.New(ctx, c.Redis)
	if err != nil {
		panic(fmt.Sprintf("failed to connect redis: %v", err))
	}

	// 派生子 context：cancel 由 ServiceContext 管理，外部 ctx 取消时子 ctx 也会取消
	ctx, cancel := context.WithCancel(ctx)

	// 事务性 Outbox 生产端：CreateOrder 在事务内写 outbox 事件，
	// Poller 异步将其发布到 order-events Stream。
	// 消费端暂不启用：当前无真实跨域消费者，等加 notification/analytics 服务时再接入。
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
		UserCli:     userCli,
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

// Stop 取消所有 goroutine 的 context 并等待它们退出。
func (sc *ServiceContext) Stop() {
	sc.cancel()
	sc.wg.Wait()
	logx.Info("all background goroutines stopped")
}

package svc

import (
	"context"
	"fmt"
	"os"
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
	Consumer    *xstream.Consumer
	RateLimiter *middleware.RateLimiter
	cancel      context.CancelFunc
	wg          sync.WaitGroup
}

func NewServiceContext(ctx context.Context, c *config.Config) *ServiceContext {
	gormDB, err := xdb.New(&c.Database)
	if err != nil {
		panic(fmt.Sprintf("failed to connect database: %v", err))
	}
	if err := xdb.Migrate(gormDB, "order"); err != nil { //nolint:contextcheck // Migrate 不接受 context 参数
		panic(fmt.Sprintf("failed to migrate: %v", err))
	}

	userCli := client.NewUserClient(&c.UserSvc)

	redisClient, err := xredis.New(c.Redis) //nolint:contextcheck // New 不接受 context 参数
	if err != nil {
		panic(fmt.Sprintf("failed to connect redis: %v", err))
	}

	// 派生子 context：cancel 由 ServiceContext 管理，外部 ctx 取消时子 ctx 也会取消
	ctx, cancel := context.WithCancel(ctx)

	// 初始化 Redis Streams 事务性 Outbox 系统
	outboxRepo := xevent.NewOutboxRepository(gormDB)
	producer := xstream.NewProducer(redisClient)
	poller := xstream.NewPoller(outboxRepo, producer, 5*time.Second, 100)

	hostname := os.Getenv("HOSTNAME")
	if hostname == "" {
		hostname = "order-api-1"
	}
	consumer := xstream.NewConsumer(
		redisClient,
		xstream.ConsumerConfig{
			Group:  "order-api",
			Stream: xevent.TopicUserEvents,
			Name:   "order-api-" + hostname,
		},
		HandleUserEvent(gormDB),
	)

	rateLimiter := middleware.NewRateLimiter(ctx, 100, time.Minute)

	sc := &ServiceContext{
		Config:      *c,
		DB:          gormDB,
		OrderRepo:   repository.NewOrderRepository(gormDB),
		UserCli:     userCli,
		Redis:       redisClient,
		OutboxRepo:  outboxRepo,
		Producer:    producer,
		Poller:      poller,
		Consumer:    consumer,
		RateLimiter: rateLimiter,
		cancel:      cancel,
	}

	poller.Start(ctx, &sc.wg)
	consumer.Start(ctx, &sc.wg)

	return sc
}

// Stop 取消所有 goroutine 的 context 并等待它们退出。
func (sc *ServiceContext) Stop() {
	sc.cancel()
	sc.wg.Wait()
	logx.Info("all background goroutines stopped")
}

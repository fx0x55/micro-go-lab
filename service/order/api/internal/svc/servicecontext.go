package svc

import (
	"context"
	"fmt"
	"sync"
	"time"
	"os"
	"strconv"

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
	Config            config.Config
	DB                *gorm.DB
	OrderRepo         repository.OrderRepositoryInterface
	UserCli           userservice.UserService
	RpcCli            zrpc.Client
	Redis             *redis.Client
	OutboxRepo        *xevent.OutboxRepository
	Producer          *xstream.Producer
	Poller            *xstream.Poller
	Consumer          *xstream.Consumer
	InventoryConsumer *xstream.Consumer
	IdempotentRepo    *xevent.IdempotentRepository
	RateLimiter       *middleware.RedisRateLimiter
	ctx               context.Context
	cancel            context.CancelFunc
	wg                sync.WaitGroup
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
	producer, err := xstream.NewProducer(c.Kafka.BootstrapServers)
	if err != nil {
		panic(fmt.Sprintf("failed to create kafka producer: %v", err))
	}
	poller := xstream.NewPoller(outboxRepo, producer, 5*time.Second, 100)

	// rateLimitPerMin 默认 100 次/分钟/IP；压测/troubleshooting lab 时用 BUG_LOAD_RATE 调高绕过。
	rateLimitPerMin := 100
	if v := os.Getenv("BUG_LOAD_RATE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			rateLimitPerMin = n
		}
	}

	rateLimiter := middleware.NewRedisRateLimiter(
		gozeroRedis.New(c.Redis.Addr(), gozeroRedis.WithPass(c.Redis.Password)),
		// 默认 100 次/分钟/IP；troubleshooting lab 压 CPU 时用 BUG_LOAD_RATE 调高（如 100000）绕过限流。
		rateLimitPerMin, time.Minute, "ratelimit:order-api:",
	)

	sc := &ServiceContext{
		Config:         *c,
		DB:             gormDB,
		OrderRepo:      repository.NewOrderRepository(gormDB),
		UserCli:        userservice.NewUserService(rpcCli),
		RpcCli:         rpcCli,
		Redis:          redisClient,
		OutboxRepo:     outboxRepo,
		Producer:       producer,
		Poller:         poller,
		IdempotentRepo: xevent.NewIdempotentRepository(gormDB),
		RateLimiter:    rateLimiter,
		ctx:            ctx,
		cancel:         cancel,
	}

	// 消费 user-events Kafka topic（CQRS：维护 known_users 物化视图）
	if c.Kafka.Topic != "" && c.Kafka.GroupID != "" {
		sc.Consumer = xstream.NewConsumer(xstream.ConsumerConfig{
			Brokers: c.Kafka.BootstrapServers,
			Topic:   c.Kafka.Topic,
			Group:   c.Kafka.GroupID,
		}, sc.handleUserEvent)
	}

	// 消费 inventory-events Kafka topic（Saga：处理库存预占结果）
	if c.Kafka.InventoryTopic != "" && c.Kafka.GroupID != "" {
		sc.InventoryConsumer = xstream.NewConsumer(xstream.ConsumerConfig{
			Brokers: c.Kafka.BootstrapServers,
			Topic:   c.Kafka.InventoryTopic,
			Group:   c.Kafka.GroupID,
		}, sc.handleInventoryEvent)
	}

	poller.Start(ctx, &sc.wg)

	return sc
}

// Start 启动后台 goroutine（Consumer）。在 main 完成 svcCtx 构造后调用。
func (sc *ServiceContext) Start() {
	if sc.Consumer != nil {
		sc.Consumer.Start(sc.ctx, &sc.wg)
	}
	if sc.InventoryConsumer != nil {
		sc.InventoryConsumer.Start(sc.ctx, &sc.wg)
	}
}

func (sc *ServiceContext) Stop() {
	sc.cancel()
	sc.wg.Wait()
	if sc.Producer != nil {
		_ = sc.Producer.Close()
	}
	logx.Info("all background goroutines stopped")
}

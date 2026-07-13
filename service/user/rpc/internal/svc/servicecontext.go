package svc

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/fx0x55/micro-go-lab/common/xcache"
	"github.com/fx0x55/micro-go-lab/common/xdb"
	"github.com/fx0x55/micro-go-lab/common/xevent"
	"github.com/fx0x55/micro-go-lab/common/xredis"
	"github.com/fx0x55/micro-go-lab/common/xstream"
	"github.com/fx0x55/micro-go-lab/service/user/rpc/internal/config"
	"github.com/fx0x55/micro-go-lab/service/user/rpc/internal/repository"
	"github.com/redis/go-redis/v9"
	"github.com/zeromicro/go-zero/core/logx"
	"gorm.io/gorm"
)

type ServiceContext struct {
	Config     config.Config
	DB         *gorm.DB
	UserRepo   *repository.UserRepository
	Redis      *redis.Client
	Cache      *xcache.Cache
	OutboxRepo *xevent.OutboxRepository
	Producer   *xstream.Producer
	Poller     *xstream.Poller
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
}

func NewServiceContext(ctx context.Context, c *config.Config) *ServiceContext {
	gormDB, err := xdb.New(&c.Database)
	if err != nil {
		panic(fmt.Sprintf("failed to connect database: %v", err))
	}
	if err := xdb.Migrate(ctx, gormDB, "user"); err != nil {
		panic(fmt.Sprintf("failed to migrate: %v", err))
	}

	// Redis：仅承担 cache 后端（限流 / idempotency gate 仍在 order-api）。
	// MQ 职责已迁移至 Kafka。RedisCache.Host 为空时退化为无缓存。
	var redisClient *redis.Client
	if c.RedisCache.Host != "" {
		var err error
		redisClient, err = xredis.New(ctx, c.RedisCache)
		if err != nil {
			logx.Errorf("failed to connect redis: %v, proceeding without cache", err)
		}
	}

	// 派生子 context：cancel 由 ServiceContext 管理，外部 ctx 取消时子 ctx 也会取消。
	ctx, cancel := context.WithCancel(ctx)

	// 事务性 Outbox 生产端：CreateUser 在事务内写 outbox 事件，
	// Poller 异步将其发布到 Kafka topic。user-rpc 现在是用户域唯一所有者，
	// 故 user-events 的发布链路收归于此。
	outboxRepo := xevent.NewOutboxRepository(gormDB)
	producer, err := xstream.NewProducer(c.Kafka.BootstrapServers)
	if err != nil {
		panic(fmt.Sprintf("failed to create kafka producer: %v", err))
	}
	poller := xstream.NewPoller(outboxRepo, producer, 5*time.Second, 100)

	return &ServiceContext{
		Config:   *c,
		DB:       gormDB,
		UserRepo: repository.NewUserRepository(gormDB),
		Redis:    redisClient,
		// redisClient 为 nil 时 xcache 退化为直接回源（本地无 Redis 仍可用）。
		Cache:      xcache.New(redisClient, "user:validate:", c.Cache.TTL, c.Cache.NegativeTTL),
		OutboxRepo: outboxRepo,
		Producer:   producer,
		Poller:     poller,
		ctx:        ctx,
		cancel:     cancel,
	}
}

// Start 启动后台 goroutine（Poller）。在 main 完成 svcCtx 构造后调用。
func (sc *ServiceContext) Start() {
	if sc.Poller != nil {
		sc.Poller.Start(sc.ctx, &sc.wg)
	}
}

// Stop 取消所有 goroutine 的 context 并等待它们退出。
func (sc *ServiceContext) Stop() {
	sc.cancel()
	sc.wg.Wait()
	if sc.Producer != nil {
		_ = sc.Producer.Close()
	}
	logx.Info("all background goroutines stopped")
}

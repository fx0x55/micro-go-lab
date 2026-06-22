package svc

import (
	"fmt"
	"os"
	"time"

	"github.com/fx0x55/micro-go-lab/common/client"
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
	Config     config.Config
	DB         *gorm.DB
	OrderRepo  repository.OrderRepositoryInterface
	UserCli    *client.UserClient
	Redis      *redis.Client
	OutboxRepo *xevent.OutboxRepository
	Producer   *xstream.Producer
	Poller     *xstream.Poller
	Consumer   *xstream.Consumer
}

func NewServiceContext(c *config.Config) *ServiceContext {
	gormDB, err := xdb.New(&c.Database)
	if err != nil {
		panic(fmt.Sprintf("failed to connect database: %v", err))
	}
	if err := xdb.Migrate(gormDB, "order"); err != nil {
		panic(fmt.Sprintf("failed to migrate: %v", err))
	}

	userCli := client.NewUserClient(&c.UserSvc)

	redisClient, err := xredis.New(c.Redis)
	if err != nil {
		panic(fmt.Sprintf("failed to connect redis: %v", err))
	}

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

	poller.Start()
	consumer.Start()

	return &ServiceContext{
		Config:     *c,
		DB:         gormDB,
		OrderRepo:  repository.NewOrderRepository(gormDB),
		UserCli:    userCli,
		Redis:      redisClient,
		OutboxRepo: outboxRepo,
		Producer:   producer,
		Poller:     poller,
		Consumer:   consumer,
	}
}

func (sc *ServiceContext) Stop() {
	if sc.Poller != nil {
		sc.Poller.Stop()
	}
	if sc.Consumer != nil {
		sc.Consumer.Stop()
	}
	logx.Info("stream poller and consumer stopped")
}

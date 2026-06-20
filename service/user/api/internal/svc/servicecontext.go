package svc

import (
	"fmt"
	"time"

	"github.com/fx0x55/micro-go-lab/common/xdb"
	"github.com/fx0x55/micro-go-lab/common/xevent"
	"github.com/fx0x55/micro-go-lab/common/xredis"
	"github.com/fx0x55/micro-go-lab/common/xstream"
	"github.com/fx0x55/micro-go-lab/service/user/api/internal/config"
	"github.com/fx0x55/micro-go-lab/service/user/api/internal/repository"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type ServiceContext struct {
	Config     config.Config
	DB         *gorm.DB
	UserRepo   repository.UserRepositoryInterface
	TodoRepo   repository.TodoRepositoryInterface
	OutboxRepo *xevent.OutboxRepository
	Producer   *xstream.Producer
	Poller     *xstream.Poller
	Consumer   *xstream.Consumer
	Redis      *redis.Client
}

func NewServiceContext(c *config.Config) *ServiceContext {
	gormDB, err := xdb.New(&c.Database)
	if err != nil {
		panic(fmt.Sprintf("failed to connect database: %v", err))
	}
	if err := xdb.Migrate(gormDB, "user"); err != nil {
		panic(fmt.Sprintf("failed to migrate: %v", err))
	}

	// 初始化 Redis
	redisClient, err := xredis.New(c.Redis)
	if err != nil {
		panic(fmt.Sprintf("failed to connect redis: %v", err))
	}

	// 初始化 Redis Streams 事务性 Outbox 系统
	outboxRepo := xevent.NewOutboxRepository(gormDB)
	producer := xstream.NewProducer(redisClient)
	poller := xstream.NewPoller(outboxRepo, producer, 5*time.Second, 100)
	consumer := xstream.NewConsumer(
		redisClient,
		xstream.ConsumerConfig{
			Group:  "user-api",
			Stream: xevent.TopicOrderEvents,
			Name:   "user-api-1",
		},
		HandleOrderEvent,
	)

	poller.Start()
	consumer.Start()

	return &ServiceContext{
		Config:     *c,
		DB:         gormDB,
		UserRepo:   repository.NewUserRepository(gormDB),
		TodoRepo:   repository.NewTodoRepository(gormDB),
		OutboxRepo: outboxRepo,
		Producer:   producer,
		Poller:     poller,
		Consumer:   consumer,
		Redis:      redisClient,
	}
}

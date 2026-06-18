package svc

import (
	"fmt"
	"time"

	"github.com/fx0x55/micro-go-lab/common/xdb"
	"github.com/fx0x55/micro-go-lab/service/user/api/internal/config"
	"github.com/fx0x55/micro-go-lab/service/user/api/internal/event"
	"github.com/fx0x55/micro-go-lab/service/user/api/internal/repository"
	"gorm.io/gorm"
)

type ServiceContext struct {
	Config   config.Config
	DB       *gorm.DB
	UserRepo repository.UserRepositoryInterface
	TodoRepo repository.TodoRepositoryInterface
	Outbox   *event.Outbox
	EventBus event.EventBus
	Consumer *event.Consumer
	Poller   *event.Poller
}

func NewServiceContext(c *config.Config) *ServiceContext {
	gormDB, err := xdb.New(&c.Database)
	if err != nil {
		panic(fmt.Sprintf("failed to connect database: %v", err))
	}
	if err := xdb.Migrate(gormDB, "user"); err != nil {
		panic(fmt.Sprintf("failed to migrate: %v", err))
	}

	// 初始化事件系统
	eventBus := event.NewChannelEventBus(100)
	outbox := event.NewOutbox(eventBus)
	consumer := event.NewConsumer(eventBus)
	poller := event.NewPoller(outbox, 5*time.Second)

	// 启动消费者和轮询器
	consumer.Start()
	poller.Start()

	return &ServiceContext{
		Config:   *c,
		DB:       gormDB,
		UserRepo: repository.NewUserRepository(gormDB),
		TodoRepo: repository.NewTodoRepository(gormDB),
		Outbox:   outbox,
		EventBus: eventBus,
		Consumer: consumer,
		Poller:   poller,
	}
}

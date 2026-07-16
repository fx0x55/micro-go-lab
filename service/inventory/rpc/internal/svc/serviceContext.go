package svc

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/fx0x55/micro-go-lab/common/xdb"
	"github.com/fx0x55/micro-go-lab/common/xevent"
	"github.com/fx0x55/micro-go-lab/common/xstream"
	"github.com/fx0x55/micro-go-lab/service/inventory/rpc/internal/config"
	"github.com/fx0x55/micro-go-lab/service/inventory/rpc/internal/repository"
	"github.com/zeromicro/go-zero/core/logx"
	"gorm.io/gorm"
)

type ServiceContext struct {
	Config         config.Config
	DB             *gorm.DB
	InventoryRepo  repository.InventoryRepositoryInterface
	OutboxRepo     *xevent.OutboxRepository
	IdempotentRepo *xevent.IdempotentRepository
	Producer       *xstream.Producer
	Poller         *xstream.Poller
	Consumer       *xstream.Consumer
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
}

func NewServiceContext(ctx context.Context, c *config.Config) *ServiceContext {
	gormDB, err := xdb.New(&c.Database)
	if err != nil {
		panic(fmt.Sprintf("failed to connect database: %v", err))
	}
	if err := xdb.Migrate(ctx, gormDB, "inventory"); err != nil {
		panic(fmt.Sprintf("failed to migrate: %v", err))
	}

	ctx, cancel := context.WithCancel(ctx)

	outboxRepo := xevent.NewOutboxRepository(gormDB)
	producer, err := xstream.NewProducer(c.Kafka.BootstrapServers)
	if err != nil {
		panic(fmt.Sprintf("failed to create kafka producer: %v", err))
	}
	poller := xstream.NewPoller(outboxRepo, producer, 5*time.Second, 100)

	sc := &ServiceContext{
		Config:         *c,
		DB:             gormDB,
		InventoryRepo:  repository.NewInventoryRepository(gormDB),
		OutboxRepo:     outboxRepo,
		IdempotentRepo: xevent.NewIdempotentRepository(gormDB),
		Producer:       producer,
		Poller:         poller,
		ctx:            ctx,
		cancel:         cancel,
	}

	// 消费 order-events：处理 OrderCreated（预占）和 OrderCancelled（释放）。
	if c.Kafka.Topic != "" && c.Kafka.GroupID != "" {
		sc.Consumer = xstream.NewConsumer(xstream.ConsumerConfig{
			Brokers: c.Kafka.BootstrapServers,
			Topic:   c.Kafka.Topic,
			Group:   c.Kafka.GroupID,
		}, sc.handleOrderEvent)
	}

	poller.Start(ctx, &sc.wg)

	return sc
}

func (sc *ServiceContext) Start() {
	if sc.Consumer != nil {
		sc.Consumer.Start(sc.ctx, &sc.wg)
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

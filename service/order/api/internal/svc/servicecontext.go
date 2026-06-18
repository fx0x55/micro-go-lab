package svc

import (
	"fmt"

	"github.com/redis/go-redis/v9"
	"github.com/zeromicro/go-zero/core/logx"
	"gorm.io/gorm"

	"github.com/wokoworks/go-server/common/client"
	"github.com/wokoworks/go-server/common/xdb"
	"github.com/wokoworks/go-server/common/xredis"
	"github.com/wokoworks/go-server/service/order/api/internal/config"
	"github.com/wokoworks/go-server/service/order/api/internal/repository"
)

type ServiceContext struct {
	Config    config.Config
	DB        *gorm.DB
	OrderRepo *repository.OrderRepository
	UserCli   *client.UserClient
	Redis     *redis.Client // 用于下单幂等；为 nil 时退化为不做幂等。
}

func NewServiceContext(c config.Config) *ServiceContext {
	gormDB, err := xdb.New(c.Database)
	if err != nil {
		panic(fmt.Sprintf("failed to connect database: %v", err))
	}
	if err := xdb.Migrate(gormDB, "order"); err != nil {
		panic(fmt.Sprintf("failed to migrate: %v", err))
	}

	userCli := client.NewUserClient(c.UserSvc)

	var redisClient *redis.Client
	if c.Redis.Host != "" {
		redisClient, err = xredis.New(c.Redis)
		if err != nil {
			logx.Errorf("failed to connect redis: %v, proceeding without idempotency", err)
		}
	}

	return &ServiceContext{
		Config:    c,
		DB:        gormDB,
		OrderRepo: repository.NewOrderRepository(gormDB),
		UserCli:   userCli,
		Redis:     redisClient,
	}
}

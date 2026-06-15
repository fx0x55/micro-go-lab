package svc

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/wokoworks/go-server/common/client"
	"github.com/wokoworks/go-server/common/xdb"
	"github.com/wokoworks/go-server/service/order/api/internal/config"
	"github.com/wokoworks/go-server/service/order/api/internal/repository"
)

type ServiceContext struct {
	Config    config.Config
	DB        *gorm.DB
	OrderRepo *repository.OrderRepository
	UserCli   *client.UserClient
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

	return &ServiceContext{
		Config:    c,
		DB:        gormDB,
		OrderRepo: repository.NewOrderRepository(gormDB),
		UserCli:   userCli,
	}
}

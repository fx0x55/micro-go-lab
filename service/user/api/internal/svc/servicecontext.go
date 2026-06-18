package svc

import (
	"fmt"

	"github.com/wokoworks/go-server/common/xdb"
	"github.com/wokoworks/go-server/service/user/api/internal/config"
	"github.com/wokoworks/go-server/service/user/api/internal/repository"
	"gorm.io/gorm"
)

type ServiceContext struct {
	Config   config.Config
	DB       *gorm.DB
	UserRepo *repository.UserRepository
	TodoRepo *repository.TodoRepository
}

func NewServiceContext(c *config.Config) *ServiceContext {
	gormDB, err := xdb.New(&c.Database)
	if err != nil {
		panic(fmt.Sprintf("failed to connect database: %v", err))
	}
	if err := xdb.Migrate(gormDB, "user"); err != nil {
		panic(fmt.Sprintf("failed to migrate: %v", err))
	}

	return &ServiceContext{
		Config:   *c,
		DB:       gormDB,
		UserRepo: repository.NewUserRepository(gormDB),
		TodoRepo: repository.NewTodoRepository(gormDB),
	}
}

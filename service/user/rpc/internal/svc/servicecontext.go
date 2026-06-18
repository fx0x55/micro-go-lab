package svc

import (
	"fmt"

	"github.com/redis/go-redis/v9"
	"github.com/zeromicro/go-zero/core/logx"
	"gorm.io/gorm"

	"github.com/wokoworks/go-server/common/xredis"
	"github.com/wokoworks/go-server/common/xdb"
	"github.com/wokoworks/go-server/service/user/rpc/internal/config"
	"github.com/wokoworks/go-server/service/user/rpc/internal/repository"
)

type ServiceContext struct {
	Config   config.Config
	DB       *gorm.DB
	UserRepo *repository.UserRepository
	Redis    *redis.Client
}

func NewServiceContext(c config.Config) *ServiceContext {
	gormDB, err := xdb.New(c.Database)
	if err != nil {
		panic(fmt.Sprintf("failed to connect database: %v", err))
	}
	if err := xdb.Migrate(gormDB, "user"); err != nil {
		panic(fmt.Sprintf("failed to migrate: %v", err))
	}

	var redisClient *redis.Client
	if c.Redis.Host != "" {
		var err error
		redisClient, err = xredis.New(c.Redis)
		if err != nil {
			logx.Errorf("failed to connect redis: %v, proceeding without cache", err)
		}
	}

	return &ServiceContext{
		Config:   c,
		DB:       gormDB,
		UserRepo: repository.NewUserRepository(gormDB),
		Redis:    redisClient,
	}
}

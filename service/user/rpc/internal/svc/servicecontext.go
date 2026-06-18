package svc

import (
	"fmt"

	"github.com/redis/go-redis/v9"
	"github.com/wokoworks/go-server/common/xcache"
	"github.com/wokoworks/go-server/common/xdb"
	"github.com/wokoworks/go-server/common/xredis"
	"github.com/wokoworks/go-server/service/user/rpc/internal/config"
	"github.com/wokoworks/go-server/service/user/rpc/internal/repository"
	"github.com/zeromicro/go-zero/core/logx"
	"gorm.io/gorm"
)

type ServiceContext struct {
	Config   config.Config
	DB       *gorm.DB
	UserRepo *repository.UserRepository
	Redis    *redis.Client
	Cache    *xcache.Cache
}

func NewServiceContext(c *config.Config) *ServiceContext {
	gormDB, err := xdb.New(&c.Database)
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
		Config:   *c,
		DB:       gormDB,
		UserRepo: repository.NewUserRepository(gormDB),
		Redis:    redisClient,
		// redisClient 为 nil 时 xcache 退化为直接回源（本地无 Redis 仍可用）。
		Cache: xcache.New(redisClient, "user:validate:", c.Cache.TTL, c.Cache.NegativeTTL),
	}
}

package main

import (
	"fmt"

	"github.com/zeromicro/go-zero/core/conf"
	goservice "github.com/zeromicro/go-zero/core/service"
	"github.com/zeromicro/go-zero/rest"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/wokoworks/go-server/internal/client"
	"github.com/wokoworks/go-server/internal/config"
	"github.com/wokoworks/go-server/internal/order/handler"
	"github.com/wokoworks/go-server/internal/order/model"
	"github.com/wokoworks/go-server/internal/order/repository"
	"github.com/wokoworks/go-server/internal/order/service"
	"github.com/wokoworks/go-server/internal/validator"
)

func main() {
	var cfg config.Config
	conf.MustLoad("config/order-svc.yaml", &cfg)
	cfg.MustSetUp()
	validator.Init()

	// Connect to user-svc via gRPC (etcd service discovery)
	userCli := client.NewUserClient(cfg.UserSvc)
	defer userCli.Close()

	// Database
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		cfg.Database.Host, cfg.Database.Port,
		cfg.Database.User, cfg.Database.Password, cfg.Database.DBName,
	)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		panic(fmt.Sprintf("failed to connect database: %v", err))
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(cfg.Database.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.Database.MaxIdleConns)

	if err := db.AutoMigrate(&model.Order{}); err != nil {
		panic(fmt.Sprintf("failed to migrate: %v", err))
	}

	// Wire dependencies
	orderRepo := repository.NewOrderRepository(db)
	orderSvc := service.NewOrderService(orderRepo, userCli)
	orderHandler := handler.NewOrderHandler(orderSvc)

	// HTTP server
	httpSrv := rest.MustNewServer(cfg.RestConf)
	defer httpSrv.Stop()
	registerHTTPRoutes(httpSrv, orderHandler, cfg)

	// Start with graceful shutdown
	group := goservice.NewServiceGroup()
	group.Add(httpSrv)
	group.Start()
}

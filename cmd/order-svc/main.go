package main

import (
	"context"
	"fmt"

	"github.com/zeromicro/go-zero/core/conf"
	goservice "github.com/zeromicro/go-zero/core/service"
	"github.com/zeromicro/go-zero/rest"

	"github.com/wokoworks/go-server/internal/client"
	"github.com/wokoworks/go-server/internal/config"
	dbx "github.com/wokoworks/go-server/internal/db"
	"github.com/wokoworks/go-server/internal/order/handler"
	"github.com/wokoworks/go-server/internal/order/repository"
	"github.com/wokoworks/go-server/internal/order/service"
	"github.com/wokoworks/go-server/internal/telemetry"
	"github.com/wokoworks/go-server/internal/validator"
)

func main() {
	var cfg config.Config
	conf.MustLoad("config/order-svc.yaml", &cfg)
	cfg.ApplyEnvOverrides()
	cfg.MustSetUp()
	validator.Init()

	// Telemetry（OpenTelemetry → Jaeger）
	shutdown, err := telemetry.Init(cfg.Name, cfg.Telemetry.OTLPEndpoint)
	if err != nil {
		panic(fmt.Sprintf("failed to init telemetry: %v", err))
	}
	defer shutdown(context.Background())

	// Connect to user-svc via gRPC (etcd service discovery)
	userCli := client.NewUserClient(cfg.UserSvc)
	defer userCli.Close()

	// Database
	gormDB, err := dbx.New(cfg.Database)
	if err != nil {
		panic(fmt.Sprintf("failed to connect database: %v", err))
	}
	if err := dbx.Migrate(gormDB, "order"); err != nil {
		panic(fmt.Sprintf("failed to migrate: %v", err))
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		panic(fmt.Sprintf("failed to get sql.DB: %v", err))
	}

	// Wire dependencies
	orderRepo := repository.NewOrderRepository(gormDB)
	orderSvc := service.NewOrderService(orderRepo, userCli)
	orderHandler := handler.NewOrderHandler(orderSvc)

	// HTTP server
	httpSrv := rest.MustNewServer(cfg.RestConf)
	defer httpSrv.Stop()
	registerHTTPRoutes(httpSrv, orderHandler, cfg, sqlDB.Ping)

	// Start with graceful shutdown
	group := goservice.NewServiceGroup()
	group.Add(httpSrv)
	group.Start()
}

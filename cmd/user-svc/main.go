package main

import (
	"fmt"

	"github.com/zeromicro/go-zero/core/conf"
	goservice "github.com/zeromicro/go-zero/core/service"
	"github.com/zeromicro/go-zero/rest"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	userv1 "github.com/wokoworks/go-server/gen/user/v1"
	"github.com/wokoworks/go-server/internal/config"
	usergrpc "github.com/wokoworks/go-server/internal/user/grpc"
	"github.com/wokoworks/go-server/internal/user/handler"
	"github.com/wokoworks/go-server/internal/user/model"
	"github.com/wokoworks/go-server/internal/user/repository"
	"github.com/wokoworks/go-server/internal/user/service"
	"github.com/wokoworks/go-server/internal/validator"
)

func main() {
	var cfg config.Config
	conf.MustLoad("config/user-svc.yaml", &cfg)
	cfg.MustSetUp()
	validator.Init()

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

	if err := db.AutoMigrate(&model.User{}, &model.Todo{}); err != nil {
		panic(fmt.Sprintf("failed to migrate: %v", err))
	}

	// Wire dependencies
	userRepo := repository.NewUserRepository(db)
	todoRepo := repository.NewTodoRepository(db)
	userSvc := service.NewUserService(userRepo, cfg.JWT)
	todoSvc := service.NewTodoService(todoRepo)
	userHandler := handler.NewUserHandler(userSvc)
	todoHandler := handler.NewTodoHandler(todoSvc)

	// HTTP server
	httpSrv := rest.MustNewServer(cfg.RestConf)
	defer httpSrv.Stop()
	registerHTTPRoutes(httpSrv, userHandler, todoHandler, cfg)

	// gRPC server
	grpcSrv := zrpc.MustNewServer(zrpc.RpcServerConf{
		ServiceConf: cfg.ServiceConf,
		ListenOn:    cfg.GRPC.ListenOn,
		Etcd:        cfg.GRPC.Etcd,
		Health:      true,
	}, func(s *grpc.Server) {
		userv1.RegisterUserServiceServer(s, usergrpc.NewUserGRPCServer(userSvc))
	})
	defer grpcSrv.Stop()

	// Start all servers with graceful shutdown
	group := goservice.NewServiceGroup()
	group.Add(httpSrv)
	group.Add(grpcSrv)
	group.Start()
}

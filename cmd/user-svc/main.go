package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	userv1 "github.com/wokoworks/go-server/gen/user/v1"
	"github.com/wokoworks/go-server/internal/config"
	"github.com/wokoworks/go-server/internal/middleware"
	"github.com/wokoworks/go-server/internal/telemetry"
	usergrpc "github.com/wokoworks/go-server/internal/user/grpc"
	"github.com/wokoworks/go-server/internal/user/handler"
	"github.com/wokoworks/go-server/internal/user/model"
	"github.com/wokoworks/go-server/internal/user/repository"
	"github.com/wokoworks/go-server/internal/user/service"
)

func main() {
	cfg := config.Load("config/user-svc.yaml")

	zapLogger, _ := zap.NewProduction()
	defer zapLogger.Sync()

	// Telemetry
	if cfg.Telemetry.Enabled {
		shutdown, err := telemetry.Init("user-svc", cfg.Telemetry.Endpoint)
		if err != nil {
			log.Fatalf("failed to init telemetry: %v", err)
		}
		defer shutdown(context.Background())
	}

	// Database
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		cfg.Database.Host, cfg.Database.Port,
		cfg.Database.User, cfg.Database.Password, cfg.Database.DBName,
	)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(cfg.Database.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.Database.MaxIdleConns)

	if err := db.AutoMigrate(&model.User{}, &model.Todo{}); err != nil {
		log.Fatalf("failed to migrate: %v", err)
	}

	// Wire dependencies
	userRepo := repository.NewUserRepository(db)
	todoRepo := repository.NewTodoRepository(db)
	userSvc := service.NewUserService(userRepo, cfg.JWT)
	todoSvc := service.NewTodoService(todoRepo)
	userHandler := handler.NewUserHandler(userSvc)
	todoHandler := handler.NewTodoHandler(todoSvc)

	// HTTP server
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.Logger(zapLogger))
	r.Use(middleware.CORS())
	r.Use(middleware.Metrics())
	r.Use(middleware.RateLimit(middleware.NewIPRateLimiter(10, 20))) // 10 req/s, burst 20

	r.GET("/health", func(c *gin.Context) {
		dbOK := true
		if sqlDB, err := db.DB(); err != nil || sqlDB.Ping() != nil {
			dbOK = false
		}
		status := "ok"
		code := 200
		if !dbOK {
			status = "degraded"
			code = 503
		}
		c.JSON(code, gin.H{"status": status, "service": "user-svc", "db": dbOK})
	})
	r.GET("/metrics", middleware.PrometheusHandler())

	auth := r.Group("/api/v1")
	{
		auth.POST("/register", userHandler.Register)
		auth.POST("/login", userHandler.Login)
	}
	api := r.Group("/api/v1")
	api.Use(middleware.JWTAuth(cfg.JWT.Secret))
	{
		api.GET("/profile", userHandler.Profile)
		api.POST("/todos", todoHandler.Create)
		api.GET("/todos", todoHandler.List)
		api.GET("/todos/:id", todoHandler.Get)
		api.PUT("/todos/:id", todoHandler.Update)
		api.DELETE("/todos/:id", todoHandler.Delete)
	}

	httpSrv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      r,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	// gRPC server
	grpcSrv := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
	)
	userv1.RegisterUserServiceServer(grpcSrv, usergrpc.NewUserGRPCServer(userSvc))
	reflection.Register(grpcSrv)

	grpcPort := cfg.Server.GRPCPort
	if grpcPort == 0 {
		grpcPort = 9090
	}

	// Start HTTP
	go func() {
		zapLogger.Info("HTTP server starting", zap.Int("port", cfg.Server.Port))
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			zapLogger.Fatal("HTTP server failed", zap.Error(err))
		}
	}()

	// Start gRPC
	go func() {
		lis, err := net.Listen("tcp", fmt.Sprintf(":%d", grpcPort))
		if err != nil {
			zapLogger.Fatal("gRPC listen failed", zap.Error(err))
		}
		zapLogger.Info("gRPC server starting", zap.Int("port", grpcPort))
		if err := grpcSrv.Serve(lis); err != nil {
			zapLogger.Fatal("gRPC server failed", zap.Error(err))
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	zapLogger.Info("shutting down...")

	grpcSrv.GracefulStop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(ctx); err != nil {
		zapLogger.Fatal("HTTP server forced shutdown", zap.Error(err))
	}

	zapLogger.Info("server exited")
}

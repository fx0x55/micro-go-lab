package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/wokoworks/go-server/internal/client"
	"github.com/wokoworks/go-server/internal/config"
	"github.com/wokoworks/go-server/internal/middleware"
	"github.com/wokoworks/go-server/internal/order/handler"
	"github.com/wokoworks/go-server/internal/order/model"
	"github.com/wokoworks/go-server/internal/order/repository"
	"github.com/wokoworks/go-server/internal/order/service"
)

func main() {
	cfg := config.Load("config/order-svc.yaml")

	zapLogger, _ := zap.NewProduction()
	defer zapLogger.Sync()

	// Connect to user-svc via gRPC
	userCli, err := client.NewUserClient(cfg.UserSvc.GRPCAddr, zapLogger)
	if err != nil {
		log.Fatalf("failed to connect user-svc: %v", err)
	}
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
		log.Fatalf("failed to connect database: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(cfg.Database.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.Database.MaxIdleConns)

	if err := db.AutoMigrate(&model.Order{}); err != nil {
		log.Fatalf("failed to migrate: %v", err)
	}

	// Wire dependencies
	orderRepo := repository.NewOrderRepository(db)
	orderSvc := service.NewOrderService(orderRepo, userCli)
	orderHandler := handler.NewOrderHandler(orderSvc)

	// HTTP server
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.Logger(zapLogger))
	r.Use(middleware.CORS())

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "order-svc"})
	})

	api := r.Group("/api/v1")
	api.Use(middleware.JWTAuth(cfg.JWT.Secret))
	{
		api.POST("/orders", orderHandler.Create)
		api.GET("/orders", orderHandler.List)
		api.GET("/orders/:id", orderHandler.Get)
		api.PUT("/orders/:id/status", orderHandler.UpdateStatus)
	}

	httpSrv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      r,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	go func() {
		zapLogger.Info("order-svc starting", zap.Int("port", cfg.Server.Port))
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			zapLogger.Fatal("server failed", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	zapLogger.Info("shutting down order-svc...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(ctx); err != nil {
		zapLogger.Fatal("server forced shutdown", zap.Error(err))
	}

	zapLogger.Info("order-svc exited")
}

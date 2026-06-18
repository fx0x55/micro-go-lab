package handler

import (
	"net/http"
	"time"

	"github.com/fx0x55/micro-go-lab/common/middleware"
	"github.com/fx0x55/micro-go-lab/service/order/api/internal/svc"
	"github.com/zeromicro/go-zero/rest"
)

func RegisterHandlers(server *rest.Server, svcCtx *svc.ServiceContext) {
	server.Use(middleware.RequestLogger)
	server.Use(middleware.NewCorsMiddleware(svcCtx.Config.CORS))
	server.Use(middleware.NewRateLimiter(100, time.Minute).Middleware)

	// Health check
	server.AddRoute(rest.Route{
		Method: http.MethodGet,
		Path:   "/health",
		Handler: middleware.HealthHandler("order-api", func() error {
			sqlDB, err := svcCtx.DB.DB()
			if err != nil {
				return err
			}
			return sqlDB.Ping()
		}),
	})

	// All order routes require JWT
	server.AddRoutes([]rest.Route{
		{Method: http.MethodPost, Path: "/api/v1/orders", Handler: CreateOrderHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/api/v1/orders", Handler: ListOrderHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/api/v1/orders/:id", Handler: GetOrderHandler(svcCtx)},
		{Method: http.MethodPut, Path: "/api/v1/orders/:id/status", Handler: UpdateOrderStatusHandler(svcCtx)},
	}, rest.WithJwt(svcCtx.Config.JWT.Secret))
}

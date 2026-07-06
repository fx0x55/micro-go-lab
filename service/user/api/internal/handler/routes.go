package handler

import (
	"net/http"

	"github.com/fx0x55/micro-go-lab/common/middleware"
	"github.com/fx0x55/micro-go-lab/service/user/api/internal/svc"
	"github.com/zeromicro/go-zero/rest"
)

func RegisterHandlers(server *rest.Server, svcCtx *svc.ServiceContext) {
	server.Use(svcCtx.RateLimiter.Middleware)

	// Health check
	server.AddRoute(rest.Route{
		Method: http.MethodGet,
		Path:   "/health",
		// 网关无 DB 依赖：健康检查只反映进程存活。
		// 下游 user-rpc 的可用性由 gRPC 客户端的熔断/重试保障，无需在此主动探测。
		Handler: middleware.HealthHandler("user-api", func() error { return nil }),
	})

	// Public routes (no JWT)
	server.AddRoutes([]rest.Route{
		{Method: http.MethodPost, Path: "/api/v1/register", Handler: RegisterHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/api/v1/login", Handler: LoginHandler(svcCtx)},
	})

	// JWT-protected routes
	server.AddRoutes([]rest.Route{
		{Method: http.MethodGet, Path: "/api/v1/profile", Handler: ProfileHandler(svcCtx)},
	}, rest.WithJwt(svcCtx.Config.JWT.Secret))
}

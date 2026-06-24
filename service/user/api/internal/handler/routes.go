package handler

import (
	"net/http"

	"github.com/fx0x55/micro-go-lab/common/middleware"
	"github.com/fx0x55/micro-go-lab/service/user/api/internal/svc"
	"github.com/zeromicro/go-zero/rest"
)

func RegisterHandlers(server *rest.Server, svcCtx *svc.ServiceContext) {
	server.Use(middleware.RequestLogger)
	server.Use(middleware.NewCorsMiddleware(svcCtx.Config.CORS))
	server.Use(svcCtx.RateLimiter.Middleware)

	// Health check
	server.AddRoute(rest.Route{
		Method: http.MethodGet,
		Path:   "/health",
		Handler: middleware.HealthHandler("user-api", func() error {
			sqlDB, err := svcCtx.DB.DB()
			if err != nil {
				return err
			}
			return sqlDB.Ping()
		}),
	})

	// Public routes (no JWT)
	server.AddRoutes([]rest.Route{
		{Method: http.MethodPost, Path: "/api/v1/register", Handler: RegisterHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/api/v1/login", Handler: LoginHandler(svcCtx)},
	})

	// JWT-protected routes
	server.AddRoutes([]rest.Route{
		{Method: http.MethodGet, Path: "/api/v1/profile", Handler: ProfileHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/api/v1/todos", Handler: CreateTodoHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/api/v1/todos", Handler: ListTodoHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/api/v1/todos/:id", Handler: GetTodoHandler(svcCtx)},
		{Method: http.MethodPut, Path: "/api/v1/todos/:id", Handler: UpdateTodoHandler(svcCtx)},
		{Method: http.MethodDelete, Path: "/api/v1/todos/:id", Handler: DeleteTodoHandler(svcCtx)},
	}, rest.WithJwt(svcCtx.Config.JWT.Secret))
}

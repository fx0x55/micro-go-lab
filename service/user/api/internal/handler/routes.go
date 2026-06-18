package handler

import (
	"net/http"

	"github.com/zeromicro/go-zero/rest"

	"github.com/wokoworks/go-server/common/middleware"
	"time"

	"github.com/wokoworks/go-server/service/user/api/internal/svc"
)

func RegisterHandlers(server *rest.Server, svcCtx *svc.ServiceContext) {
	server.Use(middleware.RequestLogger)
	server.Use(middleware.CorsMiddleware)
	server.Use(middleware.NewRateLimiter(100, time.Minute).Middleware)

	// Health check
	server.AddRoute(rest.Route{
		Method:  http.MethodGet,
		Path:    "/health",
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

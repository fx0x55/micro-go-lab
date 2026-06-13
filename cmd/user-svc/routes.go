package main

import (
	"encoding/json"
	"net/http"

	"github.com/zeromicro/go-zero/rest"

	"github.com/wokoworks/go-server/internal/config"
	"github.com/wokoworks/go-server/internal/user/handler"
)

func registerHTTPRoutes(srv *rest.Server, userH *handler.UserHandler, todoH *handler.TodoHandler, cfg config.Config) {
	// Health check
	srv.AddRoute(rest.Route{
		Method:  http.MethodGet,
		Path:    "/health",
		Handler: healthHandler,
	})

	// Public routes (no JWT)
	srv.AddRoutes([]rest.Route{
		{Method: http.MethodPost, Path: "/api/v1/register", Handler: userH.Register},
		{Method: http.MethodPost, Path: "/api/v1/login", Handler: userH.Login},
	})

	// JWT-protected routes
	srv.AddRoutes([]rest.Route{
		{Method: http.MethodGet, Path: "/api/v1/profile", Handler: userH.Profile},
		{Method: http.MethodPost, Path: "/api/v1/todos", Handler: todoH.Create},
		{Method: http.MethodGet, Path: "/api/v1/todos", Handler: todoH.List},
		{Method: http.MethodGet, Path: "/api/v1/todos/:id", Handler: todoH.Get},
		{Method: http.MethodPut, Path: "/api/v1/todos/:id", Handler: todoH.Update},
		{Method: http.MethodDelete, Path: "/api/v1/todos/:id", Handler: todoH.Delete},
	}, rest.WithJwt(cfg.JWT.Secret))
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "service": "user-svc"})
}

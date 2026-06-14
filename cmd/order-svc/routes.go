package main

import (
	"net/http"

	"github.com/zeromicro/go-zero/rest"

	"github.com/wokoworks/go-server/internal/config"
	"github.com/wokoworks/go-server/internal/middleware"
	"github.com/wokoworks/go-server/internal/order/handler"
)

func registerHTTPRoutes(srv *rest.Server, orderH *handler.OrderHandler, cfg config.Config) {
	// Health check
	srv.AddRoute(rest.Route{
		Method:  http.MethodGet,
		Path:    "/health",
		Handler: middleware.HealthHandler("order-svc"),
	})

	// All order routes require JWT
	srv.AddRoutes([]rest.Route{
		{Method: http.MethodPost, Path: "/api/v1/orders", Handler: orderH.Create},
		{Method: http.MethodGet, Path: "/api/v1/orders", Handler: orderH.List},
		{Method: http.MethodGet, Path: "/api/v1/orders/:id", Handler: orderH.Get},
		{Method: http.MethodPut, Path: "/api/v1/orders/:id/status", Handler: orderH.UpdateStatus},
	}, rest.WithJwt(cfg.JWT.Secret))
}

package main

import (
	"encoding/json"
	"net/http"

	"github.com/zeromicro/go-zero/rest"

	"github.com/wokoworks/go-server/internal/config"
	"github.com/wokoworks/go-server/internal/order/handler"
)

func registerHTTPRoutes(srv *rest.Server, orderH *handler.OrderHandler, cfg config.Config) {
	// Health check
	srv.AddRoute(rest.Route{
		Method:  http.MethodGet,
		Path:    "/health",
		Handler: healthHandler,
	})

	// All order routes require JWT
	srv.AddRoutes([]rest.Route{
		{Method: http.MethodPost, Path: "/api/v1/orders", Handler: orderH.Create},
		{Method: http.MethodGet, Path: "/api/v1/orders", Handler: orderH.List},
		{Method: http.MethodGet, Path: "/api/v1/orders/:id", Handler: orderH.Get},
		{Method: http.MethodPut, Path: "/api/v1/orders/:id/status", Handler: orderH.UpdateStatus},
	}, rest.WithJwt(cfg.JWT.Secret))
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "service": "order-svc"})
}

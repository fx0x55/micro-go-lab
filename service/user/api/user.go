package main

import (
	"flag"
	"fmt"
	"net/http"

	commonconfig "github.com/fx0x55/micro-go-lab/common/config"
	"github.com/fx0x55/micro-go-lab/common/middleware"
	"github.com/fx0x55/micro-go-lab/common/validator"
	"github.com/fx0x55/micro-go-lab/service/user/api/internal/config"
	"github.com/fx0x55/micro-go-lab/service/user/api/internal/handler"
	"github.com/fx0x55/micro-go-lab/service/user/api/internal/svc"
	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/proc"
	"github.com/zeromicro/go-zero/rest"
)

var configFile = flag.String("f", "etc/user-api.yaml", " config file")

func main() {
	flag.Parse()

	var cfg config.Config
	conf.MustLoad(*configFile, &cfg)
	cfg.ApplyEnvOverrides()
	if err := commonconfig.ValidateSecrets(cfg.Mode, cfg.Auth.AccessSecret); err != nil {
		panic(err)
	}
	cfg.MustSetUp()
	validator.Init()

	svcCtx := svc.NewServiceContext(&cfg)

	proc.AddShutdownListener(func() {
		svcCtx.Stop()
	})

	httpSrv := rest.MustNewServer(
		cfg.RestConf,
		rest.WithCors(cfg.CORS.AllowedOrigins...),
		rest.WithNotAllowedHandler(middleware.NotAllowHandler()),
	)
	defer httpSrv.Stop()

	// Q5：server 级限流 + 自定义健康检查在 main 挂载（routes.go 纯 stock 生成）
	httpSrv.Use(svcCtx.RateLimiter.Middleware)
	httpSrv.AddRoute(rest.Route{
		Method:  http.MethodGet,
		Path:    "/health",
		Handler: middleware.HealthHandler("user-api", func() error { return nil }),
	})

	handler.RegisterHandlers(httpSrv, svcCtx)

	fmt.Printf("Starting user-api server at %s:%d...\n", cfg.Host, cfg.Port)
	httpSrv.Start()
}

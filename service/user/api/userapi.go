package main

import (
	"flag"
	"fmt"

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

var configFile = flag.String("f", "etc/user-api.yaml", "the config file")

func main() {
	flag.Parse()

	var cfg config.Config
	conf.MustLoad(*configFile, &cfg)
	cfg.ApplyEnvOverrides()
	if err := commonconfig.ValidateSecrets(cfg.Mode, cfg.JWT.Secret); err != nil {
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
	handler.RegisterHandlers(httpSrv, svcCtx)

	fmt.Printf("Starting user-api server at %s:%d...\n", cfg.Host, cfg.Port)
	httpSrv.Start()
}

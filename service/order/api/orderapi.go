package main

import (
	"flag"
	"fmt"

	commonconfig "github.com/fx0x55/micro-go-lab/common/config"
	"github.com/fx0x55/micro-go-lab/common/validator"
	"github.com/fx0x55/micro-go-lab/service/order/api/internal/config"
	"github.com/fx0x55/micro-go-lab/service/order/api/internal/handler"
	"github.com/fx0x55/micro-go-lab/service/order/api/internal/svc"
	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/proc"
	"github.com/zeromicro/go-zero/rest"
)

var configFile = flag.String("f", "etc/order-api.yaml", "the config file")

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
		_ = svcCtx.UserCli.Close()
		if sqlDB, err := svcCtx.DB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	httpSrv := rest.MustNewServer(cfg.RestConf)
	defer httpSrv.Stop()
	handler.RegisterHandlers(httpSrv, svcCtx)

	fmt.Printf("Starting order-api server at %s:%d...\n", cfg.Host, cfg.Port)
	httpSrv.Start()
}

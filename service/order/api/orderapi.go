package main

import (
	"flag"
	"fmt"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/rest"

	"github.com/wokoworks/go-server/common/validator"
	"github.com/wokoworks/go-server/service/order/api/internal/config"
	"github.com/wokoworks/go-server/service/order/api/internal/handler"
	"github.com/wokoworks/go-server/service/order/api/internal/svc"
)

var configFile = flag.String("f", "etc/order-api.yaml", "the config file")

func main() {
	flag.Parse()

	var cfg config.Config
	conf.MustLoad(*configFile, &cfg)
	cfg.ApplyEnvOverrides()
	cfg.MustSetUp()
	validator.Init()

	svcCtx := svc.NewServiceContext(cfg)
	defer svcCtx.UserCli.Close()

	httpSrv := rest.MustNewServer(cfg.RestConf)
	defer httpSrv.Stop()
	handler.RegisterHandlers(httpSrv, svcCtx)

	fmt.Printf("Starting order-api server at %s:%d...\n", cfg.Host, cfg.Port)
	httpSrv.Start()
}

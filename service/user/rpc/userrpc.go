package main

import (
	"flag"
	"fmt"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/proc"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/wokoworks/go-server/service/user/rpc/internal/config"
	"github.com/wokoworks/go-server/service/user/rpc/internal/server"
	"github.com/wokoworks/go-server/service/user/rpc/internal/svc"
	userv1 "github.com/wokoworks/go-server/service/user/rpc/pb"
)

var configFile = flag.String("f", "etc/user-rpc.yaml", "the config file")

func main() {
	flag.Parse()

	var cfg config.Config
	conf.MustLoad(*configFile, &cfg)
	cfg.ApplyEnvOverrides()
	cfg.MustSetUp()

	svcCtx := svc.NewServiceContext(cfg)

	proc.AddShutdownListener(func() {
		if sqlDB, err := svcCtx.DB.DB(); err == nil {
			sqlDB.Close()
		}
	})

	s := zrpc.MustNewServer(cfg.RpcServerConf, func(grpcServer *grpc.Server) {
		userv1.RegisterUserServiceServer(grpcServer, server.NewUserServiceServer(svcCtx))
		if cfg.Mode == "dev" || cfg.Mode == "test" {
			reflection.Register(grpcServer)
		}
	})
	defer s.Stop()

	fmt.Printf("Starting user-rpc server at %s...\n", cfg.ListenOn)
	s.Start()
}

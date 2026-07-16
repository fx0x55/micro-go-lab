package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/fx0x55/micro-go-lab/service/inventory/rpc/internal/config"
	"github.com/fx0x55/micro-go-lab/service/inventory/rpc/internal/server"
	"github.com/fx0x55/micro-go-lab/service/inventory/rpc/internal/svc"
	pb "github.com/fx0x55/micro-go-lab/service/inventory/rpc/pb"
	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/proc"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var configFile = flag.String("f", "etc/inventory-rpc.yaml", "the config file")

func main() {
	flag.Parse()

	var cfg config.Config
	conf.MustLoad(*configFile, &cfg)
	cfg.ApplyEnvOverrides()
	cfg.MustSetUp()

	svcCtx := svc.NewServiceContext(context.Background(), &cfg)
	svcCtx.Start()

	proc.AddShutdownListener(func() {
		svcCtx.Stop()
		if sqlDB, err := svcCtx.DB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	s := zrpc.MustNewServer(cfg.RpcServerConf, func(grpcServer *grpc.Server) {
		pb.RegisterInventoryServiceServer(grpcServer, server.NewInventoryServiceServer(svcCtx))
		if cfg.Mode == "dev" || cfg.Mode == "test" {
			reflection.Register(grpcServer)
		}
	})
	defer s.Stop()

	fmt.Printf("Starting inventory-rpc server at %s...\n", cfg.ListenOn)
	s.Start()
}

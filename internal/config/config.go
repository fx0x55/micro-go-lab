package config

import (
	"fmt"
	"time"

	"github.com/zeromicro/go-zero/core/discov"
	"github.com/zeromicro/go-zero/core/service"
	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	service.ServiceConf
	Host     string         `json:",default=0.0.0.0"`
	Port     int            `json:",default=8080"`
	GRPCConf GRPCConfig     `json:",optional"`
	Database DatabaseConfig
	JWT      JWTConfig
	UserSvc  UserSvcConfig `json:",optional"`
}

type GRPCConfig struct {
	Port int              `json:",default=9090"`
	Etcd discov.EtcdConf `json:",optional"`
}

func (c GRPCConfig) RpcServerConf(serviceConf service.ServiceConf) zrpc.RpcServerConf {
	return zrpc.RpcServerConf{
		ServiceConf: serviceConf,
		ListenOn:    fmt.Sprintf(":%d", c.Port),
		Etcd:        c.Etcd,
		Health:      true,
	}
}

type DatabaseConfig struct {
	Host         string `json:",default=localhost"`
	Port         int    `json:",default=5432"`
	User         string
	Password     string
	DBName       string
	MaxOpenConns int `json:",default=25"`
	MaxIdleConns int `json:",default=10"`
}

type JWTConfig struct {
	Secret     string
	Expiration time.Duration `json:",default=24h"`
}

type UserSvcConfig struct {
	Etcd      discov.EtcdConf
	Endpoints []string `json:",optional"`
	Timeout   int64    `json:",default=2000"`
}

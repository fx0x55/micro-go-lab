package config

import (
	"time"

	"github.com/zeromicro/go-zero/core/discov"
	"github.com/zeromicro/go-zero/rest"
)

// Config 是两个服务共享的配置结构。
// 内嵌 rest.RestConf 已包含 ServiceConf（Log/Telemetry/Prometheus 等）与 Host/Port，
// 两个服务直接复用，避免手动拼装 RestConf。
type Config struct {
	rest.RestConf
	GRPC     GRPCConfig     `json:",optional"` // 仅 user-svc 监听 gRPC 时使用
	Database DatabaseConfig
	JWT      JWTConfig
	UserSvc  UserSvcConfig `json:",optional"` // 仅 order-svc 调用 user-svc 时使用
}

// GRPCConfig 是 gRPC 服务端监听配置，ServiceConf 复用 HTTP 侧的配置
type GRPCConfig struct {
	ListenOn string          `json:",default=:9090"`
	Etcd     discov.EtcdConf `json:",optional"`
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

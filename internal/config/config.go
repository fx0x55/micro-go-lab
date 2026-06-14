package config

import (
	"os"
	"strings"
	"time"

	"github.com/zeromicro/go-zero/core/discov"
	"github.com/zeromicro/go-zero/rest"
)

// Config 是两个服务共享的配置结构。
// 内嵌 rest.RestConf 已包含 ServiceConf（Log/Telemetry/Prometheus 等）与 Host/Port，
// 两个服务直接复用，避免手动拼装 RestConf。
type Config struct {
	rest.RestConf
	GRPC      GRPCConfig      `json:",optional"` // 仅 user-svc 监听 gRPC 时使用
	Database  DatabaseConfig
	JWT       JWTConfig
	UserSvc   UserSvcConfig   `json:",optional"` // 仅 order-svc 调用 user-svc 时使用
	Telemetry TelemetryConfig `json:",optional"`
}

// GRPCConfig 是 gRPC 服务端监听配置，ServiceConf 复用 HTTP 侧的配置
type GRPCConfig struct {
	ListenOn string          `json:",default=:9090"`
	Etcd     discov.EtcdConf `json:",optional"`
}

// TelemetryConfig 是 OpenTelemetry 链路追踪配置
type TelemetryConfig struct {
	OTLPEndpoint string `json:",optional"` // OTLP gRPC 接收端点，如 jaeger:4317
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

// ApplyEnvOverrides 用环境变量覆盖部署相关配置。
// go-zero 的 conf 默认不把环境变量绑定到字段，docker-compose 注入的环境变量
// 在此显式读取，让容器化部署覆盖 yaml 里的 localhost 默认值与敏感 secret。
func (c *Config) ApplyEnvOverrides() {
	if s := os.Getenv("JWT_SECRET"); s != "" {
		c.JWT.Secret = s
	}
	if s := os.Getenv("OTLP_ENDPOINT"); s != "" {
		c.Telemetry.OTLPEndpoint = s
	}
	if s := os.Getenv("DATABASE_HOST"); s != "" {
		c.Database.Host = s
	}
	if hosts := envList("GRPC_ETCD_HOSTS"); len(hosts) > 0 {
		c.GRPC.Etcd.Hosts = hosts
	}
	if k := os.Getenv("GRPC_ETCD_KEY"); k != "" {
		c.GRPC.Etcd.Key = k
	}
	if hosts := envList("USERSVC_ETCD_HOSTS"); len(hosts) > 0 {
		c.UserSvc.Etcd.Hosts = hosts
	}
	if k := os.Getenv("USERSVC_ETCD_KEY"); k != "" {
		c.UserSvc.Etcd.Key = k
	}
}

func envList(name string) []string {
	s := os.Getenv(name)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

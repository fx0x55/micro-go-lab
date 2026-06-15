package config

import (
	"os"
	"strings"
	"time"

	"github.com/zeromicro/go-zero/core/discov"
)

// GRPCConfig 是 gRPC 服务端监听配置
type GRPCConfig struct {
	ListenOn string          `json:",default=:9090"`
	Etcd     discov.EtcdConf `json:",optional"`
}

// TelemetryConfig 是 OpenTelemetry 链路追踪配置
type TelemetryConfig struct {
	OTLPEndpoint string `json:",optional"`
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

// EnvList 从环境变量读取逗号分隔的列表
func EnvList(name string) []string {
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

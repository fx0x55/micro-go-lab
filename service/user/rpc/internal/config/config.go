package config

import (
	"os"

	"github.com/zeromicro/go-zero/zrpc"

	"github.com/wokoworks/go-server/common/config"
)

type Config struct {
	zrpc.RpcServerConf
	Database  config.DatabaseConfig
	Telemetry config.TelemetryConfig `json:",optional"`
}

func (c *Config) ApplyEnvOverrides() {
	if s := os.Getenv("OTLP_ENDPOINT"); s != "" {
		c.Telemetry.OTLPEndpoint = s
	}
	if s := os.Getenv("DATABASE_HOST"); s != "" {
		c.Database.Host = s
	}
	if hosts := config.EnvList("ETCD_HOSTS"); len(hosts) > 0 {
		c.Etcd.Hosts = hosts
	}
	if k := os.Getenv("ETCD_KEY"); k != "" {
		c.Etcd.Key = k
	}
}

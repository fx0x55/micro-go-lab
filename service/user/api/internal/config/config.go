package config

import (
	"os"

	commonconfig "github.com/fx0x55/micro-go-lab/common/config"
	"github.com/zeromicro/go-zero/rest"
)

type Config struct {
	rest.RestConf
	Auth struct {
		AccessSecret string
		AccessExpire int64
	}
	UserSvc commonconfig.UserSvcConfig
	CORS    commonconfig.CORSConfig
	Redis   commonconfig.RedisConfig
}

func (c *Config) ApplyEnvOverrides() {
	if s := os.Getenv("JWT_SECRET"); s != "" {
		c.Auth.AccessSecret = s
	}
	if s := os.Getenv("OTLP_ENDPOINT"); s != "" {
		c.Telemetry.Endpoint = s
	}
	if hosts := commonconfig.EnvList("ETCD_HOSTS"); len(hosts) > 0 {
		c.UserSvc.Etcd.Hosts = hosts
	}
	if k := os.Getenv("ETCD_KEY"); k != "" {
		c.UserSvc.Etcd.Key = k
	}
	if s := os.Getenv("REDIS_HOST"); s != "" {
		c.Redis.Host = s
	}
	c.CORS.ApplyEnvOverrides()
}

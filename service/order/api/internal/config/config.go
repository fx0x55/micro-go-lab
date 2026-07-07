package config

import (
	"os"
	"strconv"

	commonconfig "github.com/fx0x55/micro-go-lab/common/config"
	"github.com/zeromicro/go-zero/rest"
)

type Config struct {
	rest.RestConf
	Auth struct {
		AccessSecret string
		AccessExpire int64
	}
	Database commonconfig.DatabaseConfig
	UserSvc  commonconfig.UserSvcConfig
	Redis    commonconfig.RedisConfig
	CORS     commonconfig.CORSConfig
}

func (c *Config) ApplyEnvOverrides() {
	if s := os.Getenv("JWT_SECRET"); s != "" {
		c.Auth.AccessSecret = s
	}
	if s := os.Getenv("OTLP_ENDPOINT"); s != "" {
		c.Telemetry.Endpoint = s
	}
	if s := os.Getenv("DATABASE_HOST"); s != "" {
		c.Database.Host = s
	}
	if s := os.Getenv("DATABASE_PORT"); s != "" {
		if port, err := strconv.Atoi(s); err == nil {
			c.Database.Port = port
		}
	}
	c.Database.ApplyEnvOverrides()
	if hosts := commonconfig.EnvList("USERSVC_ETCD_HOSTS"); len(hosts) > 0 {
		c.UserSvc.Etcd.Hosts = hosts
	}
	if k := os.Getenv("USERSVC_ETCD_KEY"); k != "" {
		c.UserSvc.Etcd.Key = k
	}
	if s := os.Getenv("REDIS_HOST"); s != "" {
		c.Redis.Host = s
	}
	c.CORS.ApplyEnvOverrides()
}

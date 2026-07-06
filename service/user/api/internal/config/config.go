package config

import (
	"os"

	"github.com/fx0x55/micro-go-lab/common/config"
	"github.com/zeromicro/go-zero/rest"
)

// user-api 是纯 HTTP 网关：不持有数据库，所有用户数据通过 user-rpc 访问。
type Config struct {
	rest.RestConf
	UserSvc config.UserSvcConfig // user-rpc 服务发现
	JWT     config.JWTConfig
	CORS    config.CORSConfig
	Redis   config.RedisConfig // 限流后端
}

func (c *Config) ApplyEnvOverrides() {
	if s := os.Getenv("JWT_SECRET"); s != "" {
		c.JWT.Secret = s
	}
	if s := os.Getenv("OTLP_ENDPOINT"); s != "" {
		c.Telemetry.Endpoint = s
	}
	if hosts := config.EnvList("ETCD_HOSTS"); len(hosts) > 0 {
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

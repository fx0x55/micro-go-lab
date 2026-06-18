package config

import (
	"os"
	"strconv"
	"time"

	"github.com/wokoworks/go-server/common/config"
	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	zrpc.RpcServerConf
	Database config.DatabaseConfig
	Redis    config.RedisConfig
	Cache    config.CacheConfig
}

func (c *Config) ApplyEnvOverrides() {
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
	if hosts := config.EnvList("ETCD_HOSTS"); len(hosts) > 0 {
		c.Etcd.Hosts = hosts
	}
	if k := os.Getenv("ETCD_KEY"); k != "" {
		c.Etcd.Key = k
	}
	if s := os.Getenv("REDIS_HOST"); s != "" {
		c.Redis.Host = s
	}
	if s := os.Getenv("CACHE_TTL"); s != "" {
		if dur, err := time.ParseDuration(s); err == nil {
			c.Cache.TTL = dur
		}
	}
	if s := os.Getenv("CACHE_NEGATIVE_TTL"); s != "" {
		if dur, err := time.ParseDuration(s); err == nil {
			c.Cache.NegativeTTL = dur
		}
	}
}

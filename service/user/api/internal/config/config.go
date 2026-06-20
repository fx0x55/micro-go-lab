package config

import (
	"os"
	"strconv"

	"github.com/fx0x55/micro-go-lab/common/config"
	"github.com/zeromicro/go-zero/rest"
)

type Config struct {
	rest.RestConf
	Database config.DatabaseConfig
	JWT      config.JWTConfig
	CORS     config.CORSConfig
	Redis    config.RedisConfig
}

func (c *Config) ApplyEnvOverrides() {
	if s := os.Getenv("JWT_SECRET"); s != "" {
		c.JWT.Secret = s
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
	if s := os.Getenv("REDIS_HOST"); s != "" {
		c.Redis.Host = s
	}
	c.Database.ApplyEnvOverrides()
	c.CORS.ApplyEnvOverrides()
}

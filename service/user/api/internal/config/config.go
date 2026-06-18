package config

import (
	"os"
	"strconv"

	"github.com/zeromicro/go-zero/rest"

	"github.com/wokoworks/go-server/common/config"
)

type Config struct {
	rest.RestConf
	Database config.DatabaseConfig
	JWT      config.JWTConfig
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
	c.Database.ApplyEnvOverrides()
}

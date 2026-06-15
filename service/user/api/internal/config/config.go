package config

import (
	"os"

	"github.com/zeromicro/go-zero/rest"

	"github.com/wokoworks/go-server/common/config"
)

type Config struct {
	rest.RestConf
	Database  config.DatabaseConfig
	JWT       config.JWTConfig
	Telemetry config.TelemetryConfig `json:",optional"`
}

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
}

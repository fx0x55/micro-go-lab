package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/zeromicro/go-zero/core/discov"
)

// RedisConfig 是 Redis 连接配置。
type RedisConfig struct {
	Host     string `json:",default=localhost"`
	Port     int    `json:",default=6379"`
	Password string `json:",optional"`
	DB       int    `json:",default=0"`
}

// Addr 返回 host:port 格式地址。
func (r *RedisConfig) Addr() string {
	return fmt.Sprintf("%s:%d", r.Host, r.Port)
}

// GRPCConfig 是 gRPC 服务端监听配置
type GRPCConfig struct {
	ListenOn string          `json:",default=:9090"`
	Etcd     discov.EtcdConf `json:",optional"`
}

type DatabaseConfig struct {
	Host         string `json:",default=localhost"`
	Port         int    `json:",default=5432"`
	User         string
	Password     string
	DBName       string
	MaxOpenConns int           `json:",default=25"`
	MaxIdleConns int           `json:",default=10"`
	// ConnMaxLifetime 限制单个连接的最长存活时间，防止连接老化
	// （DB 重启 / LB 轮换 / NAT 超时后陈旧连接）。
	ConnMaxLifetime time.Duration `json:",default=30m"`
	// ConnMaxIdleTime 限制连接最长空闲时间，回收池中暂时不用的连接。
	ConnMaxIdleTime time.Duration `json:",default=5m"`
	// SSLMode 透传 PostgreSQL sslmode；空字符串等同于 disable（保留本地开发默认）。
	// 生产建议 require 或 verify-full。
	SSLMode string `json:",optional"`
}

// ApplyEnvOverrides 从环境变量读取连接池与 TLS 相关配置，供各服务 config 复用。
// 解析失败的字段保持原值（通常是 YAML 默认值）。
func (d *DatabaseConfig) ApplyEnvOverrides() {
	if s := os.Getenv("DATABASE_SSLMODE"); s != "" {
		d.SSLMode = s
	}
	if s := os.Getenv("DATABASE_CONN_MAX_LIFETIME"); s != "" {
		if dur, err := time.ParseDuration(s); err == nil {
			d.ConnMaxLifetime = dur
		}
	}
	if s := os.Getenv("DATABASE_CONN_MAX_IDLE_TIME"); s != "" {
		if dur, err := time.ParseDuration(s); err == nil {
			d.ConnMaxIdleTime = dur
		}
	}
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

// CacheConfig 是 cache-aside 缓存的通用 TTL 配置。
type CacheConfig struct {
	// TTL 正缓存（命中存在的数据）的存活时间。
	TTL time.Duration `json:",default=5m"`
	// NegativeTTL 负缓存（数据确实不存在）的存活时间，通常较短以避免陈旧。
	NegativeTTL time.Duration `json:",default=30s"`
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

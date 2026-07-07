package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/zeromicro/go-zero/core/discov"
	"github.com/zeromicro/go-zero/zrpc"
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
	Port         int    `json:",default=3306"`
	User         string
	Password     string
	DBName       string
	MaxOpenConns int `json:",default=25"`
	MaxIdleConns int `json:",default=10"`
	// ConnMaxLifetime 限制单个连接的最长存活时间，防止连接老化
	// （DB 重启 / LB 轮换 / NAT 超时后陈旧连接）。
	ConnMaxLifetime time.Duration `json:",default=30m"`
	// ConnMaxIdleTime 限制连接最长空闲时间，回收池中暂时不用的连接。
	ConnMaxIdleTime time.Duration `json:",default=5m"`
	// SlowThreshold 慢查询阈值；超过此时间的 SQL 查询将被记录。
	// 设为 0 时不记录慢查询。
	SlowThreshold time.Duration `json:",default=200ms"`
}

// ApplyEnvOverrides 从环境变量读取连接池相关配置，供各服务 config 复用。
// 解析失败的字段保持原值（通常是 YAML 默认值）。
func (d *DatabaseConfig) ApplyEnvOverrides() {
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
	if s := os.Getenv("DATABASE_SLOW_THRESHOLD"); s != "" {
		if dur, err := time.ParseDuration(s); err == nil {
			d.SlowThreshold = dur
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

// RpcClientConf 把 UserSvcConfig 转成 go-zero 的 zrpc.RpcClientConf。
//
// 关键：直接构造 RpcClientConf 字面量不会走 conf 解析，Middlewares 上的
// `json:",default=true"` 标签不生效，会被当成零值 false，进而导致
// Trace/Breaker/Timeout 等客户端拦截器被 buildUnaryInterceptors 跳过。
// 显式把 Middlewares 各项置 true，与 RpcServerConf 经 conf.MustLoad 后的默认行为一致。
func (c *UserSvcConfig) RpcClientConf() zrpc.RpcClientConf {
	return zrpc.RpcClientConf{
		Etcd:      c.Etcd,
		Endpoints: c.Endpoints,
		Timeout:   c.Timeout,
		NonBlock:  true,
		Middlewares: zrpc.ClientMiddlewaresConf{
			Trace:      true,
			Duration:   true,
			Prometheus: true,
			Breaker:    true,
			Timeout:    true,
		},
	}
}

// CacheConfig 是 cache-aside 缓存的通用 TTL 配置。
type CacheConfig struct {
	// TTL 正缓存（命中存在的数据）的存活时间。
	TTL time.Duration `json:",default=5m"`
	// NegativeTTL 负缓存（数据确实不存在）的存活时间，通常较短以避免陈旧。
	NegativeTTL time.Duration `json:",default=30s"`
}

// CORSConfig 是 CORS 跨域配置。
// AllowedOrigins 为 ["*"] 时允许所有来源（开发默认）；否则仅允许白名单中的来源。
type CORSConfig struct {
	AllowedOrigins []string `json:",default=[*]"`
}

// ApplyEnvOverrides 从环境变量读取 CORS 允许的来源列表（逗号分隔）。
func (c *CORSConfig) ApplyEnvOverrides() {
	if origins := EnvList("CORS_ALLOWED_ORIGINS"); len(origins) > 0 {
		c.AllowedOrigins = origins
	}
}

// defaultSecrets 是已知的不安全默认值列表。
var defaultSecrets = []string{"change-me-in-production", ""}

// ValidateSecrets 检查已知的不安全默认密钥。
// dev/test 模式下仅输出警告；pro/pre 模式下直接拒绝启动。
func ValidateSecrets(mode string, jwtSecrets ...string) error {
	var warnings []string
	for _, s := range jwtSecrets {
		for _, d := range defaultSecrets {
			if s == d {
				warnings = append(warnings, "JWT secret 使用了默认值或为空")
			}
		}
	}
	if len(warnings) == 0 {
		return nil
	}
	msg := "insecure defaults detected: " + strings.Join(warnings, "; ")
	if mode == "pro" || mode == "pre" {
		return fmt.Errorf("%s (set proper values or use dev/test mode)", msg)
	}
	// dev/test: 警告但不阻断
	fmt.Printf("WARNING: %s (acceptable in %s mode)\n", msg, mode)
	return nil
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

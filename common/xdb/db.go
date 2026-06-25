package xdb

import (
	"fmt"

	"github.com/fx0x55/micro-go-lab/common/config"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// New 初始化 MySQL 连接并设置连接池参数
func New(cfg *config.DatabaseConfig) (*gorm.DB, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true&charset=utf8mb4&loc=Local",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.DBName)
	gormDB, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: NewGormLogger(cfg.SlowThreshold),
	})
	if err != nil {
		return nil, fmt.Errorf("connect database: %w", err)
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		return nil, fmt.Errorf("get underlying sql.DB: %w", err)
	}
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	// 连接生命周期：定期回收，避免连接老化导致 DB 重启/LB 轮换后陈旧连接报错。
	if cfg.ConnMaxLifetime > 0 {
		sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	}
	if cfg.ConnMaxIdleTime > 0 {
		sqlDB.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)
	}

	// 注册 DB 连接池指标收集器（go_sql_* 指标）。
	// 使用数据库名称（如 "users_db"）作为 label，使多个数据库实例在 Prometheus 中可区分。
	prometheus.MustRegister(collectors.NewDBStatsCollector(sqlDB, cfg.DBName))

	return gormDB, nil
}

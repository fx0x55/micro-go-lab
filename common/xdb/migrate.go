package xdb

import (
	"context"
	"embed"
	"io/fs"

	"github.com/pressly/goose/v3"
	"github.com/zeromicro/go-zero/core/logx"
	"gorm.io/gorm"
)

//go:embed migrations
var migrationsFS embed.FS

// Migrate 在给定连接上执行指定服务的 SQL 迁移（goose Up）。
// goose 通过 goose_db_version 表保证幂等性，无需外部锁。
func Migrate(ctx context.Context, gormDB *gorm.DB, service string) error {
	sqlDB, err := gormDB.DB()
	if err != nil {
		return err
	}

	// 从嵌入的 FS 中取出对应服务的子目录（如 migrations/user/）。
	subFS, err := fs.Sub(migrationsFS, "migrations/"+service)
	if err != nil {
		return err
	}

	provider, err := goose.NewProvider(
		goose.DialectMySQL,
		sqlDB,
		subFS,
	)
	if err != nil {
		return err
	}
	// 注意：不能 defer provider.Close()，因为 goose v3 的 Provider.Close()
	// 会调用底层 sql.DB.Close()，关闭整个连接池，导致后续查询报
	// "sql: database is closed"。
	results, err := provider.Up(ctx)
	if err != nil {
		return err
	}
	for _, r := range results {
		logx.Infof("[goose] %s", r)
	}
	return nil
}

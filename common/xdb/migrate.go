package xdb

import (
	"context"
	"embed"
	"io/fs"

	"github.com/pressly/goose/v3"
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
	defer func() {
		_ = provider.Close()
	}()

	_, err = provider.Up(ctx)
	return err
}

package xdb

import (
	"context"
	"embed"
	"io/fs"

	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
	"gorm.io/gorm"
)

//go:embed migrations
var migrationsFS embed.FS

// Migrate 在给定连接上执行指定服务的 SQL 迁移（goose Up）。
// 使用 PostgreSQL advisory lock 防止多个服务实例并发迁移导致冲突。
func Migrate(gormDB *gorm.DB, service string) error {
	sqlDB, err := gormDB.DB()
	if err != nil {
		return err
	}

	// 从嵌入的 FS 中取出对应服务的子目录（如 migrations/user/）。
	subFS, err := fs.Sub(migrationsFS, "migrations/"+service)
	if err != nil {
		return err
	}

	// 使用 PostgreSQL advisory lock 防止并发迁移。
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return err
	}

	provider, err := goose.NewProvider(
		goose.DialectPostgres,
		sqlDB,
		subFS,
		goose.WithSessionLocker(locker),
	)
	if err != nil {
		return err
	}

	_, err = provider.Up(context.Background())
	return err
}

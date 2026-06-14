package db

import (
	"context"
	"embed"

	"github.com/pressly/goose/v3"
	"gorm.io/gorm"
)

//go:embed migrations
var migrationsFS embed.FS

// Migrate 在给定连接上执行指定服务的 SQL 迁移（goose Up）。
// service 决定使用 migrations/<service>/ 下的迁移文件。
func Migrate(gormDB *gorm.DB, service string) error {
	goose.SetBaseFS(migrationsFS)
	sqlDB, err := gormDB.DB()
	if err != nil {
		return err
	}
	return goose.UpContext(context.Background(), sqlDB, "migrations/"+service)
}

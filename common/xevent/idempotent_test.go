//go:build integration

package xevent

import (
	"errors"
	"testing"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "root:root@tcp(localhost:3306)/users_db?parseTime=true&charset=utf8mb4&loc=Local"
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	// 确保表存在
	db.AutoMigrate(&ProcessedEvent{})
	t.Cleanup(func() {
		db.Exec("DELETE FROM processed_events")
	})
	return db
}

func TestIdempotentRepository_Process(t *testing.T) {
	db := setupTestDB(t)
	repo := NewIdempotentRepository(db)

	t.Run("first call processes event", func(t *testing.T) {
		called := false
		processed, err := repo.Process("evt-001", func(tx *gorm.DB) error {
			called = true
			return nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !processed {
			t.Fatal("expected processed=true")
		}
		if !called {
			t.Fatal("expected fn to be called")
		}
	})

	t.Run("duplicate event is skipped", func(t *testing.T) {
		called := false
		processed, err := repo.Process("evt-001", func(tx *gorm.DB) error {
			called = true
			return nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if processed {
			t.Fatal("expected processed=false for duplicate")
		}
		if called {
			t.Fatal("expected fn to NOT be called for duplicate")
		}
	})

	t.Run("new event id is processed", func(t *testing.T) {
		processed, err := repo.Process("evt-002", func(tx *gorm.DB) error {
			return nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !processed {
			t.Fatal("expected processed=true for new id")
		}
	})

	t.Run("fn error rolls back transaction", func(t *testing.T) {
		processed, err := repo.Process("evt-rollback", func(tx *gorm.DB) error {
			return errors.New("business error")
		})
		if err == nil {
			t.Fatal("expected error")
		}
		if processed {
			t.Fatal("expected processed=false on error")
		}

		// 重试同 ID 应该能成功（上次失败没写入 processed_events）
		processed, err = repo.Process("evt-rollback", func(tx *gorm.DB) error {
			return nil
		})
		if err != nil {
			t.Fatalf("retry unexpected error: %v", err)
		}
		if !processed {
			t.Fatal("expected processed=true on retry after failure")
		}
	})
}

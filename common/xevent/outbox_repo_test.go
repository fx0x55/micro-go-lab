//go:build integration

package xevent

import (
	"testing"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func setupOutboxTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "root:root@tcp(localhost:3306)/users_db?parseTime=true&charset=utf8mb4&loc=Local"
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	db.AutoMigrate(&OutboxEvent{})
	t.Cleanup(func() {
		db.Exec("DELETE FROM outbox_events")
	})
	return db
}

func TestOutboxRepository_Insert(t *testing.T) {
	db := setupOutboxTestDB(t)
	repo := NewOutboxRepository(db)

	event := &OutboxEvent{
		EventID:   "test-insert-001",
		Topic:     "test-topic",
		EventKey:  "key-1",
		EventType: "test.event",
		Version:   1,
		Payload:   `{"data":"test"}`,
		Status:    OutboxStatusPending,
	}

	err := repo.Insert(db, event)
	if err != nil {
		t.Fatalf("Insert failed: %v", err)
	}
	if event.ID == 0 {
		t.Fatal("expected auto-generated ID")
	}
}

func TestOutboxRepository_MarkAsSent(t *testing.T) {
	db := setupOutboxTestDB(t)
	repo := NewOutboxRepository(db)

	event := &OutboxEvent{
		EventID:   "test-sent-001",
		Topic:     "test-topic",
		EventKey:  "key-1",
		EventType: "test.event",
		Version:   1,
		Payload:   `{}`,
		Status:    OutboxStatusPending,
	}
	if err := repo.Insert(db, event); err != nil {
		t.Fatalf("Insert failed: %v", err)
	}

	if err := repo.MarkAsSent(db, event.ID); err != nil {
		t.Fatalf("MarkAsSent failed: %v", err)
	}

	var found OutboxEvent
	db.First(&found, event.ID)
	if found.Status != OutboxStatusSent {
		t.Fatalf("expected status=sent, got %s", found.Status)
	}
	if found.SentAt == nil {
		t.Fatal("expected SentAt to be set")
	}
}

func TestOutboxRepository_MarkAsFailed(t *testing.T) {
	db := setupOutboxTestDB(t)
	repo := NewOutboxRepository(db)

	event := &OutboxEvent{
		EventID:   "test-failed-001",
		Topic:     "test-topic",
		EventKey:  "key-1",
		EventType: "test.event",
		Version:   1,
		Payload:   `{}`,
		Status:    OutboxStatusPending,
	}
	if err := repo.Insert(db, event); err != nil {
		t.Fatalf("Insert failed: %v", err)
	}

	if err := repo.MarkAsFailed(event.ID, "something broke"); err != nil {
		t.Fatalf("MarkAsFailed failed: %v", err)
	}

	var found OutboxEvent
	db.First(&found, event.ID)
	if found.Status != OutboxStatusFailed {
		t.Fatalf("expected status=failed, got %s", found.Status)
	}
	if found.LastError != "something broke" {
		t.Fatalf("expected last_error='something broke', got %s", found.LastError)
	}
}

func TestOutboxRepository_IncrementRetryCount(t *testing.T) {
	db := setupOutboxTestDB(t)
	repo := NewOutboxRepository(db)

	event := &OutboxEvent{
		EventID:   "test-retry-001",
		Topic:     "test-topic",
		EventKey:  "key-1",
		EventType: "test.event",
		Version:   1,
		Payload:   `{}`,
		Status:    OutboxStatusPending,
	}
	if err := repo.Insert(db, event); err != nil {
		t.Fatalf("Insert failed: %v", err)
	}

	for i := 1; i <= 3; i++ {
		if err := repo.IncrementRetryCount(db, event.ID, "error"); err != nil {
			t.Fatalf("IncrementRetryCount %d failed: %v", i, err)
		}
	}

	var found OutboxEvent
	db.First(&found, event.ID)
	if found.RetryCount != 3 {
		t.Fatalf("expected retry_count=3, got %d", found.RetryCount)
	}
}

func TestOutboxRepository_FindPending(t *testing.T) {
	db := setupOutboxTestDB(t)
	repo := NewOutboxRepository(db)

	// 插入 pending 事件
	for i := range 3 {
		repo.Insert(db, &OutboxEvent{
			EventID:   "pending-" + string(rune('0'+i)),
			Topic:     "t",
			EventKey:  "k",
			EventType: "e",
			Version:   1,
			Payload:   "{}",
			Status:    OutboxStatusPending,
		})
	}
	// 插入已发送事件
	repo.Insert(db, &OutboxEvent{
		EventID:   "already-sent",
		Topic:     "t",
		EventKey:  "k",
		EventType: "e",
		Version:   1,
		Payload:   "{}",
		Status:    OutboxStatusSent,
	})
	// 插入超过重试次数的事件
	repo.Insert(db, &OutboxEvent{
		EventID:   "max-retries",
		Topic:     "t",
		EventKey:  "k",
		EventType: "e",
		Version:   1,
		Payload:   "{}",
		Status:    OutboxStatusPending,
		RetryCount: MaxRetries,
	})

	events, err := repo.FindPending(10)
	if err != nil {
		t.Fatalf("FindPending failed: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 pending events, got %d", len(events))
	}

	// 测试 limit
	events, err = repo.FindPending(2)
	if err != nil {
		t.Fatalf("FindPending with limit failed: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events with limit, got %d", len(events))
	}
}

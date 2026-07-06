//go:build integration

package xstream

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func newTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Fatalf("redis not available: %v", err)
	}
	t.Cleanup(func() { rdb.Close() })
	return rdb
}

func cleanupStream(t *testing.T, rdb *redis.Client, stream string) {
	t.Helper()
	rdb.Del(context.Background(), stream)
}

func TestProducerPublish(t *testing.T) {
	rdb := newTestRedis(t)
	stream := "test-producer-" + t.Name()
	cleanupStream(t, rdb, stream)
	t.Cleanup(func() { cleanupStream(t, rdb, stream) })

	p := NewProducer(rdb)
	id, err := p.Publish(context.Background(), stream, map[string]string{
		"event":    "test.event",
		"event_id": "test-001",
		"payload":  `{"hello":"world"}`,
	})
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty message ID")
	}

	length, err := rdb.XLen(context.Background(), stream).Result()
	if err != nil {
		t.Fatalf("XLen failed: %v", err)
	}
	if length != 1 {
		t.Fatalf("expected 1 message, got %d", length)
	}
	t.Logf("Publish OK, message ID: %s, stream length: %d", id, length)
}

func TestConsumerStartReceiveMessages(t *testing.T) {
	rdb := newTestRedis(t)
	stream := "test-start-" + t.Name()
	cleanupStream(t, rdb, stream)
	t.Cleanup(func() { cleanupStream(t, rdb, stream) })

	var mu sync.Mutex
	var received []map[string]string

	handler := func(values map[string]string) error {
		mu.Lock()
		defer mu.Unlock()
		received = append(received, values)
		t.Logf("Handler received: event_id=%s", values["event_id"])
		return nil
	}

	consumer := NewConsumer(rdb, ConsumerConfig{
		Group:  "start-group",
		Stream: stream,
		Name:   "start-consumer-1",
	}, handler)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	consumer.Start(ctx, &wg)
	defer func() { cancel(); wg.Wait() }()

	// 等 consumer goroutine 启动并进入 XReadGroup 阻塞
	time.Sleep(500 * time.Millisecond)

	// 发消息
	p := NewProducer(rdb)
	for i := range 5 {
		_, err := p.Publish(context.Background(), stream, map[string]string{
			"event_id": "start-evt-" + string(rune('0'+i)),
			"payload":  "hello",
		})
		if err != nil {
			t.Fatalf("Publish %d failed: %v", i, err)
		}
	}

	// 等待 consumer 处理
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(received)
		mu.Unlock()
		if n >= 5 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	mu.Lock()
	n := len(received)
	mu.Unlock()

	if n != 5 {
		t.Fatalf("expected 5 messages consumed, got %d", n)
	}
	t.Logf("Consumer processed all %d messages", n)
}

func TestConsumerClaimPending(t *testing.T) {
	rdb := newTestRedis(t)
	stream := "test-claim-" + t.Name()
	group := "claim-group"
	cleanupStream(t, rdb, stream)
	t.Cleanup(func() { cleanupStream(t, rdb, stream) })

	// Step 1: 创建 consumer group 并发送消息
	p := NewProducer(rdb)
	for i := range 3 {
		_, err := p.Publish(context.Background(), stream, map[string]string{
			"event_id": "claim-evt-" + string(rune('0'+i)),
			"payload":  "data",
		})
		if err != nil {
			t.Fatalf("Publish %d failed: %v", i, err)
		}
	}

	err := rdb.XGroupCreateMkStream(context.Background(), stream, group, "0").Err()
	if err != nil {
		t.Fatalf("XGroupCreateMkStream failed: %v", err)
	}

	// Step 2: 用 "0" 读消息（读 pending），模拟旧 consumer 读了但没 ACK
	msgs, err := rdb.XReadGroup(context.Background(), &redis.XReadGroupArgs{
		Group:    group,
		Consumer: "old-consumer",
		Streams:  []string{stream, "0"},
		Count:    10,
		Block:    1 * time.Second,
	}).Result()
	if err != nil {
		t.Fatalf("XReadGroup '0' failed: %v", err)
	}
	totalOld := 0
	for _, s := range msgs {
		totalOld += len(s.Messages)
	}
	if totalOld != 3 {
		t.Fatalf("expected 3 messages, got %d", totalOld)
	}
	t.Log("Old consumer read 3 messages but DID NOT ack (simulating crash)")

	// 验证 pending list 有 3 条
	pending, err := rdb.XPending(context.Background(), stream, group).Result()
	if err != nil {
		t.Fatalf("XPending failed: %v", err)
	}
	if pending.Count != 3 {
		t.Fatalf("expected 3 pending, got %d", pending.Count)
	}

	// Step 3: 新 consumer 启动，应该通过 XAUTOCLAIM 认领 pending 消息
	var mu sync.Mutex
	var received []map[string]string

	handler := func(values map[string]string) error {
		mu.Lock()
		defer mu.Unlock()
		received = append(received, values)
		return nil
	}

	consumer := NewConsumer(rdb, ConsumerConfig{
		Group:  group,
		Stream: stream,
		Name:   "new-consumer",
	}, handler)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	consumer.Start(ctx, &wg)
	defer func() { cancel(); wg.Wait() }()

	// 等待 XAUTOCLAIM 处理
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(received)
		mu.Unlock()
		if n >= 3 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	mu.Lock()
	n := len(received)
	mu.Unlock()

	if n != 3 {
		t.Fatalf("expected 3 pending messages claimed, got %d", n)
	}
	t.Logf("New consumer claimed all %d pending messages via XAUTOCLAIM", n)
}

func TestConsumerReceiveAfterRestart(t *testing.T) {
	rdb := newTestRedis(t)
	stream := "test-restart-" + t.Name()
	group := "restart-group"
	cleanupStream(t, rdb, stream)
	t.Cleanup(func() { cleanupStream(t, rdb, stream) })

	p := NewProducer(rdb)
	_, err := p.Publish(context.Background(), stream, map[string]string{"event_id": "first"})
	if err != nil {
		t.Fatal(err)
	}

	err = rdb.XGroupCreateMkStream(context.Background(), stream, group, "0").Err()
	if err != nil {
		t.Fatal(err)
	}

	msgs, err := rdb.XReadGroup(context.Background(), &redis.XReadGroupArgs{
		Group: group, Consumer: "c1",
		Streams: []string{stream, "0"}, Count: 10, Block: 1 * time.Second,
	}).Result()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range msgs {
		for _, m := range s.Messages {
			_ = rdb.XAck(context.Background(), stream, group, m.ID).Err()
		}
	}
	t.Log("First message consumed and ACKed")

	// 第二次消费（模拟重启后）
	_, err = p.Publish(context.Background(), stream, map[string]string{"event_id": "second"})
	if err != nil {
		t.Fatal(err)
	}

	msgs2, err := rdb.XReadGroup(context.Background(), &redis.XReadGroupArgs{
		Group: group, Consumer: "c1",
		Streams: []string{stream, ">"}, Count: 10, Block: 1 * time.Second,
	}).Result()
	if err != nil {
		t.Fatal(err)
	}

	total := 0
	for _, s := range msgs2 {
		total += len(s.Messages)
		for _, m := range s.Messages {
			t.Logf("  Received: event_id=%s", m.Values["event_id"])
		}
	}
	if total != 1 {
		t.Fatalf("expected 1 new message after restart, got %d", total)
	}
	t.Log("Only the new message was delivered")
}

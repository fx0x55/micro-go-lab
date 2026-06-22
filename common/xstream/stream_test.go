//go:build ignore

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

// TestProducerPublish 验证 XADD 写入消息
func TestProducerPublish(t *testing.T) {
	rdb := newTestRedis(t)
	stream := "test-producer-" + t.Name()
	cleanupStream(t, rdb, stream)
	t.Cleanup(func() { cleanupStream(t, rdb, stream) })

	p := NewProducer(rdb)
	id, err := p.Publish(stream, map[string]string{
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

// TestConsumerReceivePrePublished 模拟核心场景：
// 先 publish，再启动 consumer，验证 ">" 能收到积压消息
func TestConsumerReceivePrePublished(t *testing.T) {
	rdb := newTestRedis(t)
	stream := "test-pre-" + t.Name()
	group := "pre-group"
	consumerName := "pre-consumer-1"
	cleanupStream(t, rdb, stream)
	t.Cleanup(func() { cleanupStream(t, rdb, stream) })

	// Step 1: 先发 3 条消息（模拟 Poller 在 Consumer 启动前发布）
	p := NewProducer(rdb)
	for i := range 3 {
		_, err := p.Publish(stream, map[string]string{
			"event":    "user.registered",
			"event_id": "evt-" + string(rune('0'+i)),
			"index":    string(rune('0' + i)),
		})
		if err != nil {
			t.Fatalf("Publish %d failed: %v", i, err)
		}
	}
	t.Log("Published 3 messages BEFORE consumer started")

	// Step 2: 创建 consumer group（ID=0 表示从头开始）
	err := rdb.XGroupCreateMkStream(context.Background(), stream, group, "0").Err()
	if err != nil {
		t.Fatalf("XGroupCreateMkStream failed: %v", err)
	}
	t.Log("Consumer group created with start ID 0")

	// Step 3: 用 ">" 读取 — 应该能收到 pre-publish 的消息
	// ">" 只返回 "未被任何 consumer 投递过" 的消息
	// group 刚创建时，所有消息虽然在 PEL 中但 delivery_count=0，所以 ">" 可见
	msgs, err := rdb.XReadGroup(context.Background(), &redis.XReadGroupArgs{
		Group:    group,
		Consumer: consumerName,
		Streams:  []string{stream, ">"},
		Count:    10,
		Block:    1 * time.Second,
	}).Result()
	if err != nil {
		t.Fatalf("XReadGroup '>' failed: %v", err)
	}

	total := 0
	for _, s := range msgs {
		total += len(s.Messages)
		t.Log("Received messages:")
		for _, m := range s.Messages {
			t.Logf("  ID=%s values=%v", m.ID, m.Values)
		}
	}
	if total != 3 {
		t.Fatalf("expected 3 messages with '>', got %d", total)
	}
	t.Logf("XREADGROUP '>' returned all %d pre-published messages", total)

	// Step 4: ACK 所有消息
	for _, s := range msgs {
		for _, m := range s.Messages {
			err := rdb.XAck(context.Background(), stream, group, m.ID).Err()
			if err != nil {
				t.Fatalf("XAck %s failed: %v", m.ID, err)
			}
		}
	}
	t.Log("All messages ACKed")

	// Step 5: 确认 pending 为空
	pending, err := rdb.XPending(context.Background(), stream, group).Result()
	if err != nil {
		t.Fatalf("XPending failed: %v", err)
	}
	if pending.Count != 0 {
		t.Fatalf("expected 0 pending after ACK, got %d", pending.Count)
	}
	t.Logf("Pending count: %d (all ACKed)", pending.Count)

	// Step 6: 再读一次，应该阻塞到超时返回 redis.Nil
	start := time.Now()
	_, err = rdb.XReadGroup(context.Background(), &redis.XReadGroupArgs{
		Group:    group,
		Consumer: consumerName,
		Streams:  []string{stream, ">"},
		Count:    10,
		Block:    1 * time.Second,
	}).Result()
	elapsed := time.Since(start)
	if err != redis.Nil {
		t.Fatalf("expected redis.Nil after ACK, got: %v (elapsed: %v)", err, elapsed)
	}
	t.Logf("Second read blocked for %v then returned redis.Nil (correct)", elapsed)
}

// TestConsumerStartReceiveMessages 验证 Consumer.Start() 启动后能收到消息
func TestConsumerStartReceiveMessages(t *testing.T) {
	rdb := newTestRedis(t)
	stream := "test-start-" + t.Name()
	cleanupStream(t, rdb, stream)
	t.Cleanup(func() {
		rdb.Del(context.Background(), stream)
	})

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
	consumer.Start()
	defer consumer.Stop()

	// 等 consumer goroutine 启动并进入 XReadGroup 阻塞
	time.Sleep(500 * time.Millisecond)

	// 发消息
	p := NewProducer(rdb)
	for i := range 5 {
		_, err := p.Publish(stream, map[string]string{
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

// TestConsumerReceiveAfterRestart 模拟 consumer 重启场景：
// 发消息 → ACK → 再发新消息 → 验证只收到新消息
func TestConsumerReceiveAfterRestart(t *testing.T) {
	rdb := newTestRedis(t)
	stream := "test-restart-" + t.Name()
	group := "restart-group"
	cleanupStream(t, rdb, stream)
	t.Cleanup(func() { cleanupStream(t, rdb, stream) })

	// 第一次消费
	p := NewProducer(rdb)
	_, err := p.Publish(stream, map[string]string{"event_id": "first"})
	if err != nil {
		t.Fatal(err)
	}

	err = rdb.XGroupCreateMkStream(context.Background(), stream, group, "0").Err()
	if err != nil {
		t.Fatal(err)
	}

	msgs, err := rdb.XReadGroup(context.Background(), &redis.XReadGroupArgs{
		Group: group, Consumer: "c1",
		Streams: []string{stream, ">"}, Count: 10, Block: 1 * time.Second,
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
	_, err = p.Publish(stream, map[string]string{"event_id": "second"})
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

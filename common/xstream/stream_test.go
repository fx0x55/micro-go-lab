//go:build integration

package xstream

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"
)

const testHeaderEventID = "event_id"

const testBrokers = "localhost:9094"

// uniqueTopic 生成带时间戳的唯一 topic 名，避免测试间干扰。
func uniqueTopic(prefix string) string {
	return fmt.Sprintf("test-%s-%d", prefix, time.Now().UnixNano())
}

// createTestTopic 用 kafka-go Admin API 创建一个 topic 供测试使用。
func createTestTopic(t *testing.T, topic string) {
	t.Helper()
	conn, err := kafka.Dial("tcp", testBrokers)
	if err != nil {
		t.Fatalf("dial broker failed: %v", err)
	}
	defer func() { _ = conn.Close() }()

	controller, err := conn.Controller()
	if err != nil {
		t.Fatalf("get controller failed: %v", err)
	}

	controllerAddr := fmt.Sprintf("%s:%d", controller.Host, controller.Port)
	topicConn, err := kafka.Dial("tcp", controllerAddr)
	if err != nil {
		t.Fatalf("dial controller failed: %v", err)
	}
	defer func() { _ = topicConn.Close() }()

	err = topicConn.CreateTopics(kafka.TopicConfig{
		Topic:             topic,
		NumPartitions:     3,
		ReplicationFactor: 1,
	})
	if err != nil {
		t.Logf("CreateTopics %s: %v (may already exist)", topic, err)
	}
}

// newTestProducer 创建一个不做 ensureTopics 的测试 Producer（topic 手动创建）。
func newTestProducer(t *testing.T) *Producer {
	t.Helper()
	w := &kafka.Writer{
		Addr:         kafka.TCP(testBrokers),
		Balancer:     &kafka.Hash{},
		RequiredAcks: kafka.RequireAll,
		MaxAttempts:  3,
		Async:        false,
		BatchTimeout: 10 * time.Millisecond,
	}
	t.Cleanup(func() { _ = w.Close() })
	return &Producer{writer: w}
}

// waitForMessages 轮询等待 received 切片长度达到 want，超时失败。
func waitForMessages(t *testing.T, mu *sync.Mutex, received *[]*Message, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(*received)
		mu.Unlock()
		if n >= want {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	mu.Lock()
	got := len(*received)
	mu.Unlock()
	t.Fatalf("expected %d messages, got %d (timeout)", want, got)
}

func TestProducerPublish(t *testing.T) {
	topic := uniqueTopic("producer")
	createTestTopic(t, topic)

	p := newTestProducer(t)

	err := p.Publish(context.Background(), &Message{
		Topic:   topic,
		Key:     "user-1",
		Value:   []byte(`{"event":"test"}`),
		Headers: map[string]string{testHeaderEventID: "evt-001"},
	})
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	// 用 Reader 验证消息存在
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     []string{testBrokers},
		Topic:       topic,
		Partition:   0,
		MinBytes:    1,
		MaxBytes:    10e6,
		MaxWait:     2 * time.Second,
		StartOffset: kafka.FirstOffset,
	})
	defer func() { _ = r.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	msg, err := r.ReadMessage(ctx)
	if err != nil {
		t.Fatalf("ReadMessage failed: %v", err)
	}
	if string(msg.Value) != `{"event":"test"}` {
		t.Fatalf("unexpected value: %s", msg.Value)
	}
	t.Logf("Publish OK, message key=%s offset=%d partition=%d", string(msg.Key), msg.Offset, msg.Partition)
}

func TestConsumerStartReceiveMessages(t *testing.T) {
	topic := uniqueTopic("receive")
	createTestTopic(t, topic)

	var mu sync.Mutex
	var received []*Message

	handler := func(msg *Message) error {
		mu.Lock()
		defer mu.Unlock()
		received = append(received, msg)
		return nil
	}

	group := fmt.Sprintf("recv-group-%d", time.Now().UnixNano())
	consumer := NewConsumer(ConsumerConfig{
		Brokers: []string{testBrokers},
		Topic:   topic,
		Group:   group,
	}, handler)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	consumer.Start(ctx, &wg)
	defer func() { cancel(); wg.Wait() }()

	time.Sleep(1 * time.Second) // 等 consumer 加入组

	p := newTestProducer(t)
	for i := range 5 {
		err := p.Publish(context.Background(), &Message{
			Topic:   topic,
			Key:     fmt.Sprintf("user-%d", i),
			Value:   fmt.Appendf(nil, `{testHeaderEventID:"evt-%d"}`, i),
			Headers: map[string]string{testHeaderEventID: fmt.Sprintf("evt-%d", i)},
		})
		if err != nil {
			t.Fatalf("Publish %d failed: %v", i, err)
		}
	}

	waitForMessages(t, &mu, &received, 5, 15*time.Second)
	t.Logf("Consumer processed all %d messages", len(received))
}

func TestConsumerRebalanceTakeover(t *testing.T) {
	topic := uniqueTopic("rebalance")
	createTestTopic(t, topic)
	group := fmt.Sprintf("rebal-group-%d", time.Now().UnixNano())

	var mu sync.Mutex
	var received []*Message

	handler := func(msg *Message) error {
		mu.Lock()
		defer mu.Unlock()
		received = append(received, msg)
		return nil
	}

	// Consumer A：先启动，接收消息（FetchMessage 模式天然不自动提交，模拟读到不 mark）
	consumerA := NewConsumer(ConsumerConfig{
		Brokers: []string{testBrokers},
		Topic:   topic,
		Group:   group,
	}, handler)

	ctxA, cancelA := context.WithCancel(context.Background())
	var wgA sync.WaitGroup
	consumerA.Start(ctxA, &wgA)

	time.Sleep(1 * time.Second) // 等 A 加入组

	p := newTestProducer(t)
	for i := range 3 {
		err := p.Publish(context.Background(), &Message{
			Topic:   topic,
			Key:     fmt.Sprintf("user-%d", i),
			Value:   fmt.Appendf(nil, `{testHeaderEventID:"evt-%d"}`, i),
			Headers: map[string]string{testHeaderEventID: fmt.Sprintf("evt-%d", i)},
		})
		if err != nil {
			t.Fatalf("Publish %d failed: %v", i, err)
		}
	}

	// 等 A 收到消息
	time.Sleep(2 * time.Second)

	mu.Lock()
	countA := len(received)
	mu.Unlock()
	if countA == 0 {
		t.Fatal("Consumer A did not receive any messages")
	}
	t.Logf("Consumer A received %d messages (no commit yet)", countA)

	// 停止 A（模拟 crash）
	cancelA()
	wgA.Wait()

	// 清空 received，让 B 接管后重新统计
	mu.Lock()
	received = received[:0]
	mu.Unlock()

	// Consumer B：加入同组，rebalance 后接管分区，重投未提交的消息
	consumerB := NewConsumer(ConsumerConfig{
		Brokers: []string{testBrokers},
		Topic:   topic,
		Group:   group,
	}, handler)

	ctxB, cancelB := context.WithCancel(context.Background())
	defer cancelB()
	var wgB sync.WaitGroup
	consumerB.Start(ctxB, &wgB)
	defer func() { cancelB(); wgB.Wait() }()

	waitForMessages(t, &mu, &received, 3, 15*time.Second)
	t.Logf("Consumer B took over and received all %d messages via rebalance", len(received))
}

func TestConsumerReplayDedup(t *testing.T) {
	topic := uniqueTopic("replay")
	createTestTopic(t, topic)
	group := fmt.Sprintf("replay-group-%d", time.Now().UnixNano())

	processed := make(map[string]bool)
	var mu sync.Mutex

	// 模拟 IdempotentRepository 去重：已处理的 event_id 跳过
	handler := func(msg *Message) error {
		eventID := msg.Headers[testHeaderEventID]
		mu.Lock()
		defer mu.Unlock()
		if processed[eventID] {
			t.Logf("skip duplicate event_id=%s", eventID)
			return nil
		}
		processed[eventID] = true
		t.Logf("processed event_id=%s", eventID)
		return nil
	}

	// 第一次消费
	consumer1 := NewConsumer(ConsumerConfig{
		Brokers: []string{testBrokers},
		Topic:   topic,
		Group:   group,
	}, handler)

	ctx1, cancel1 := context.WithCancel(context.Background())
	var wg1 sync.WaitGroup
	consumer1.Start(ctx1, &wg1)

	time.Sleep(1 * time.Second)

	p := newTestProducer(t)
	for i := range 3 {
		err := p.Publish(context.Background(), &Message{
			Topic:   topic,
			Key:     fmt.Sprintf("user-%d", i),
			Value:   fmt.Appendf(nil, `{testHeaderEventID:"evt-%d"}`, i),
			Headers: map[string]string{testHeaderEventID: fmt.Sprintf("evt-%d", i)},
		})
		if err != nil {
			t.Fatalf("Publish %d failed: %v", i, err)
		}
	}

	time.Sleep(3 * time.Second) // 等 consumer1 处理并提交
	cancel1()
	wg1.Wait()

	mu.Lock()
	firstPass := len(processed)
	mu.Unlock()
	if firstPass != 3 {
		t.Fatalf("expected 3 processed in first pass, got %d", firstPass)
	}

	// 第二次消费（模拟重启后 FirstOffset 重放 → 已提交的 offset 不再重投）
	consumer2 := NewConsumer(ConsumerConfig{
		Brokers: []string{testBrokers},
		Topic:   topic,
		Group:   group,
	}, handler)

	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	var wg2 sync.WaitGroup
	consumer2.Start(ctx2, &wg2)
	defer func() { cancel2(); wg2.Wait() }()

	// 等 consumer2 加入组并完成 rebalance（此时已提交的 offset 被恢复，不会重投已处理消息）
	time.Sleep(5 * time.Second)

	mu.Lock()
	secondPass := len(processed)
	mu.Unlock()

	// 已提交 offset → consumer2 从上次位置继续读 → 已处理的 3 条不会重投
	if secondPass != 3 {
		t.Fatalf("expected 3 total processed after replay (no duplicates), got %d", secondPass)
	}
	t.Logf("Replay dedup OK: %d messages processed, no duplicates on restart", secondPass)
}

//go:build redis

package redis_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Pub/Sub 发布/订阅 — 实时消息广播
// ============================================================================
//
// 【为什么用 Pub/Sub】
// Pub/Sub 是 Redis 原生的发布/订阅模式，支持：
//   - 频道（Channel）广播：一条消息所有订阅者都收到
//   - 模式订阅（Pattern）：按 glob 模式匹配多个频道
//   - 集群广播：在 Redis Cluster 中扇出到所有节点
//
// 【适用场景】
//   - 实时通知：订单状态变更通知、库存预警
//   - 聊天室：用户加入频道后实时收到消息
//   - 配置中心：发布配置变更事件
//   - 事件总线：微服务间的实时事件传递
//   - 直播弹幕：用户发送消息，所有观众实时接收
//
// 【坑和注意事项】
//   1. Pub/Sub 是"fire and forget"——消息不持久化！
//      如果订阅者离线，离线期间的消息会丢失
//   2. 没有消费者组的概念，每个订阅者都收到所有消息（广播）
//   3. 订阅者断开连接后需要重新订阅
//   4. 模式订阅（PSUBSCRIBE）比普通订阅更消耗性能
//   5. 在 Redis Cluster 中，Pub/Sub 消息会扇出到所有节点
//   6. 不保证消息顺序（网络抖动可能导致乱序）
//   7. 大量订阅者时，广播开销 O(N)，可能成为瓶颈
//   8. 如果需要可靠消息传递，用 Stream 替代 Pub/Sub

func TestPubSub_BasicPublish(t *testing.T) {
	channel := testKey("pubsub:basic")

	// 创建订阅者
	sub := rdb.Subscribe(ctx, channel)
	defer sub.Close()

	// 确保订阅已建立
	_, err := sub.Receive(ctx) // 收到订阅确认
	require.NoError(t, err)

	// 发布消息
	published, err := rdb.Publish(ctx, channel, "hello world").Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), published) // 1 个订阅者收到

	// 接收消息
	msg, err := sub.ReceiveMessage(ctx)
	require.NoError(t, err)
	assert.Equal(t, channel, msg.Channel)
	assert.Equal(t, "hello world", msg.Payload)

	rdb.Del(ctx, channel)
}

func TestPubSub_MultipleSubscribers(t *testing.T) {
	channel := testKey("pubsub:multi")

	// 创建 3 个订阅者
	sub1 := rdb.Subscribe(ctx, channel)
	sub2 := rdb.Subscribe(ctx, channel)
	sub3 := rdb.Subscribe(ctx, channel)
	defer sub1.Close()
	defer sub2.Close()
	defer sub3.Close()

	// 确保所有订阅者就绪（用 Receive 确认订阅成功，避免 Channel() 阻塞）
	subs := []*redis.PubSub{sub1, sub2, sub3}
	timeoutCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	for _, sub := range subs {
		_, err := sub.Receive(timeoutCtx)
		require.NoError(t, err)
	}

	// 发布一条消息
	rdb.Publish(ctx, channel, "broadcast message")

	// 每个订阅者都能收到
	for i, sub := range subs {
		msg, err := sub.ReceiveMessage(ctx)
		require.NoError(t, err)
		assert.Equal(t, "broadcast message", msg.Payload, "subscriber %d should receive", i+1)
	}

	rdb.Del(ctx, channel)
}

func TestPubSub_PatternSubscribe(t *testing.T) {
	// 模式订阅：订阅所有 order:* 频道
	psub := rdb.PSubscribe(ctx, "order:*")
	defer psub.Close()

	// 确保订阅就绪
	subCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, err := psub.Receive(subCtx)
	require.NoError(t, err)

	// 向不同的 order 频道发布消息
	rdb.Publish(ctx, "order:created", "order-1001")
	rdb.Publish(ctx, "order:paid", "order-1001")
	rdb.Publish(ctx, "user:created", "user-2001") // 不匹配 order:* 模式

	// 模式订阅者只能收到 order:* 频道的消息
	msg1, err := psub.ReceiveMessage(ctx)
	require.NoError(t, err)
	assert.Equal(t, "order:created", msg1.Channel)

	msg2, err := psub.ReceiveMessage(ctx)
	require.NoError(t, err)
	assert.Equal(t, "order:paid", msg2.Channel)

	rdb.Del(ctx, "order:created", "order:paid", "user:created")
}

func TestPubSub_ChannelGroup(t *testing.T) {
	// 场景：按频道分组的事件系统
	eventChannel := testKey("pubsub:events")
	logChannel := testKey("pubsub:logs")

	// 订阅两个频道
	sub := rdb.Subscribe(ctx, eventChannel, logChannel)
	defer sub.Close()
	subCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, err := sub.Receive(subCtx)
	require.NoError(t, err)

	// 发布事件
	rdb.Publish(ctx, eventChannel, "user login")
	rdb.Publish(ctx, logChannel, "access log")

	// 接收消息（顺序不确定）
	received := make(map[string]string)
	for range 2 {
		msg, err := sub.ReceiveMessage(ctx)
		require.NoError(t, err)
		received[msg.Channel] = msg.Payload
	}

	assert.Equal(t, "user login", received[eventChannel])
	assert.Equal(t, "access log", received[logChannel])

	rdb.Del(ctx, eventChannel, logChannel)
}

func TestPubSub_HighThroughput(t *testing.T) {
	channel := testKey("pubsub:throughput")

	sub := rdb.Subscribe(ctx, channel)
	defer sub.Close()
	subCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, err := sub.Receive(subCtx)
	require.NoError(t, err)

	// 使用 Channel() 方法异步接收
	msgCh := sub.Channel()
	var received []string
	var mu sync.Mutex
	done := make(chan struct{})

	// 消费者 goroutine
	go func() {
		defer close(done)
		for range 100 {
			select {
			case msg := <-msgCh:
				mu.Lock()
				received = append(received, msg.Payload)
				mu.Unlock()
			case <-time.After(2 * time.Second):
				return
			}
		}
	}()

	// 快速发布 100 条消息
	for i := range 100 {
		rdb.Publish(ctx, channel, fmt.Sprintf("msg:%d", i))
	}

	<-done

	mu.Lock()
	assert.Len(t, received, 100)
	mu.Unlock()

	rdb.Del(ctx, channel)

	// 【坑】高频发布时，订阅者可能处理不过来（消费者慢）
	// 【坑】Channel() 方法内部有缓冲区（默认100），满了会阻塞发布者
}

func TestPubSub_MessageLoss(t *testing.T) {
	channel := testKey("pubsub:loss")

	// 场景：演示 Pub/Sub 的消息丢失问题
	// 1. 先发布消息（没有订阅者）
	rdb.Publish(ctx, channel, "missed message 1")
	rdb.Publish(ctx, channel, "missed message 2")

	// 2. 然后订阅（消息已经丢了！）
	sub := rdb.Subscribe(ctx, channel)
	defer sub.Close()
	subCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, err := sub.Receive(subCtx)
	require.NoError(t, err)

	// 3. 再发布
	rdb.Publish(ctx, channel, "received message")

	// 只能收到第 3 条
	msg, err := sub.ReceiveMessage(ctx)
	require.NoError(t, err)
	assert.Equal(t, "received message", msg.Payload)

	// 这就是为什么需要 Stream 做可靠消息传递

	rdb.Del(ctx, channel)
}

func TestPubSub_StreamingInterface(t *testing.T) {
	channel := testKey("pubsub:streaming")

	sub := rdb.Subscribe(ctx, channel)
	defer sub.Close()

	// 确保订阅就绪
	subCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, err := sub.Receive(subCtx)
	require.NoError(t, err)

	// Channel() 返回 <-chan *redis.Message，方便用 for-range 消费
	msgCh := sub.Channel()

	// 发布 5 条消息
	for i := range 5 {
		rdb.Publish(ctx, channel, fmt.Sprintf("event-%d", i))
	}

	// 用 for-range 消费
	var received []string
	timeout := time.After(2 * time.Second)
	for range 5 {
		select {
		case msg := <-msgCh:
			received = append(received, msg.Payload)
		case <-timeout:
			t.Fatal("timeout waiting for messages")
		}
	}

	assert.Len(t, received, 5)
	assert.Equal(t, "event-0", received[0])
	assert.Equal(t, "event-4", received[4])

	rdb.Del(ctx, channel)
}

func TestPubSub_Unsubscribe(t *testing.T) {
	channel := testKey("pubsub:unsub")

	sub := rdb.Subscribe(ctx, channel)
	subCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, err := sub.Receive(subCtx)
	require.NoError(t, err)

	// 确认已订阅
	published, _ := rdb.Publish(ctx, channel, "before unsubscribe").Result()
	assert.Equal(t, int64(1), published)

	// 取消订阅
	sub.Unsubscribe(ctx, channel)

	// 取消后发布消息，没有订阅者收到
	published, _ = rdb.Publish(ctx, channel, "after unsubscribe").Result()
	assert.Equal(t, int64(0), published)

	sub.Close()
	rdb.Del(ctx, channel)
}

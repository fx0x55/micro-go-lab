package event

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOutbox_Add(t *testing.T) {
	eventBus := NewChannelEventBus(10)
	outbox := NewOutbox(eventBus)

	// 测试添加事件
	event1 := NewEvent(UserRegistered, map[string]any{"user_id": 1})
	outbox.Add(event1)

	event2 := NewEvent(UserRegistered, map[string]any{"user_id": 2})
	outbox.Add(event2)

	// 验证事件被正确添加
	pending := outbox.GetPending()
	assert.Len(t, pending, 2, "应该有2个待处理事件")

	// 验证事件ID递增
	assert.Equal(t, uint(1), pending[0].ID, "第一个事件ID应该是1")
	assert.Equal(t, uint(2), pending[1].ID, "第二个事件ID应该是2")

	// 验证事件状态
	assert.Equal(t, "pending", pending[0].Status, "事件状态应该是pending")
	assert.Equal(t, "pending", pending[1].Status, "事件状态应该是pending")
}

func TestOutbox_MarkAsSent(t *testing.T) {
	eventBus := NewChannelEventBus(10)
	outbox := NewOutbox(eventBus)

	// 添加事件
	event1 := NewEvent(UserRegistered, map[string]any{"user_id": 1})
	outbox.Add(event1)

	// 标记为已发送
	outbox.MarkAsSent(1)

	// 验证事件状态已更新
	pending := outbox.GetPending()
	assert.Len(t, pending, 0, "不应该有待处理事件")

	// 验证事件被标记为已发送（需要直接访问events slice）
	outbox.mu.RLock()
	assert.Equal(t, "sent", outbox.events[0].Status, "事件状态应该是sent")
	assert.False(t, outbox.events[0].SentAt.IsZero(), "SentAt应该被设置")
	outbox.mu.RUnlock()
}

func TestOutbox_MarkAsFailed(t *testing.T) {
	eventBus := NewChannelEventBus(10)
	outbox := NewOutbox(eventBus)

	// 添加事件
	event1 := NewEvent(UserRegistered, map[string]any{"user_id": 1})
	outbox.Add(event1)

	// 标记为失败
	outbox.MarkAsFailed(1)

	// 验证事件状态已更新
	pending := outbox.GetPending()
	assert.Len(t, pending, 0, "不应该有待处理事件")

	// 验证事件被标记为失败
	outbox.mu.RLock()
	assert.Equal(t, "failed", outbox.events[0].Status, "事件状态应该是failed")
	outbox.mu.RUnlock()
}

func TestOutbox_PublishPending(t *testing.T) {
	eventBus := NewChannelEventBus(10)
	outbox := NewOutbox(eventBus)

	// 记录发布的事件
	var publishedEvents []Event
	var mu sync.Mutex

	// 订阅事件
	eventBus.Subscribe(func(event Event) {
		mu.Lock()
		defer mu.Unlock()
		publishedEvents = append(publishedEvents, event)
	})

	// 添加事件
	event1 := NewEvent(UserRegistered, map[string]any{"user_id": 1})
	outbox.Add(event1)

	event2 := NewEvent(UserRegistered, map[string]any{"user_id": 2})
	outbox.Add(event2)

	// 发布待处理事件
	published := outbox.PublishPending()
	assert.Equal(t, 2, published, "应该发布2个事件")

	// 等待事件被消费
	time.Sleep(100 * time.Millisecond)

	// 验证事件被发布
	mu.Lock()
	assert.Len(t, publishedEvents, 2, "应该收到2个事件")
	mu.Unlock()

	// 验证所有事件都被标记为已发送
	pending := outbox.GetPending()
	assert.Len(t, pending, 0, "不应该有待处理事件")
}

func TestOutbox_ConcurrentAccess(t *testing.T) {
	eventBus := NewChannelEventBus(100)
	outbox := NewOutbox(eventBus)

	var wg sync.WaitGroup
	numEvents := 100

	// 并发添加事件
	for i := 0; i < numEvents; i++ {
		wg.Add(1)
		go func(userID int) {
			defer wg.Done()
			event := NewEvent(UserRegistered, map[string]any{"user_id": userID})
			outbox.Add(event)
		}(i)
	}

	wg.Wait()

	// 验证所有事件都被添加
	pending := outbox.GetPending()
	assert.Len(t, pending, numEvents, "应该有100个待处理事件")

	// 验证事件ID是唯一的
	ids := make(map[uint]bool)
	for _, event := range pending {
		assert.False(t, ids[event.ID], "事件ID应该唯一")
		ids[event.ID] = true
	}
}

func TestEventBus_PublishSubscribe(t *testing.T) {
	eventBus := NewChannelEventBus(10)

	var receivedEvents []Event
	var mu sync.Mutex

	// 订阅事件
	eventBus.Subscribe(func(event Event) {
		mu.Lock()
		defer mu.Unlock()
		receivedEvents = append(receivedEvents, event)
	})

	// 发布事件
	event1 := NewEvent(UserRegistered, map[string]any{"user_id": 1})
	eventBus.Publish(event1)

	event2 := NewEvent(UserRegistered, map[string]any{"user_id": 2})
	eventBus.Publish(event2)

	// 等待事件被消费
	time.Sleep(100 * time.Millisecond)

	// 验证事件被正确接收
	mu.Lock()
	require.Len(t, receivedEvents, 2, "应该收到2个事件")
	assert.Equal(t, event1.Type, receivedEvents[0].Type, "事件类型应该匹配")
	assert.Equal(t, event2.Type, receivedEvents[1].Type, "事件类型应该匹配")
	mu.Unlock()
}

func TestPoller_StartStop(t *testing.T) {
	eventBus := NewChannelEventBus(10)
	outbox := NewOutbox(eventBus)
	poller := NewPoller(outbox, 100*time.Millisecond)

	// 启动轮询器
	poller.Start()

	// 添加事件
	event1 := NewEvent(UserRegistered, map[string]any{"user_id": 1})
	outbox.Add(event1)

	// 等待轮询器处理
	time.Sleep(200 * time.Millisecond)

	// 验证事件被发布（通过检查Outbox状态）
	pending := outbox.GetPending()
	assert.Len(t, pending, 0, "事件应该已经被发布")

	// 停止轮询器
	poller.Stop()
}

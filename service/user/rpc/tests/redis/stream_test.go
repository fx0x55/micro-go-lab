//go:build redis

package redis_test

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const seqKey = "seq"

// ============================================================================
// Stream 流 — Redis 5.0+ 的持久化消息队列
// ============================================================================
//
// 【为什么用 Stream】
// Stream 是 Redis 原生的消息队列，支持：
//   - 持久化：消息写入磁盘（AOF/RDB），不像 List 弹出即丢失
//   - 消费者组（Consumer Group）：支持负载均衡、消息确认、pending 管理
//   - 消息 ID：自动生成时间戳 ID，天然有序
//   - 回溯消费：可以重新读取历史消息
//
// 【适用场景】
//   - 事件溯源（Event Sourcing）：用户行为事件流
//   - 微服务间消息传递：替代 Kafka（轻量场景）
//   - 任务队列：支持 ACK、重试、死信
//   - 实时数据管道：日志收集、指标上报
//   - 通知推送：新订单通知、库存预警
//
// 【坑和注意事项】
//   1. Stream 不支持按内容搜索，只能按 ID 范围读取
//   2. MAXLEN 只是"尽力删除"（~近似），不是精确删除
//   3. 消费者组的 PEL（Pending Entries List）需要手动管理，忘记 XACK 会导致消息堆积
//   4. XREADGROUP BLOCK 是阻塞操作，连接数多了会占用大量 Redis 连接
//   5. Stream 不支持事务（MULTI/EXEC），XADD + XACK 不是原子操作
//   6. 大 Stream（百万消息）的 XRANGE 命令会非常慢
//   7. 消费者组创建后不能修改，只能删除重建
//   8. 单消费者组内的消息是"竞争消费"（每条消息只被一个消费者处理）

func TestStream_BasicPublish(t *testing.T) {
	streamKey := testKey("stream:events")

	// XADD —— 写入消息
	// ID 用 * 让 Redis 自动生成（时间戳-序列号格式）
	id1, err := rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: streamKey,
		Values: map[string]string{
			"event": "user.registered",
			"uid":   "1001",
			"name":  "张三",
		},
	}).Result()
	require.NoError(t, err)
	assert.NotEmpty(t, id1) // ID 格式：1688000000000-0

	// XLEN —— 消息数量
	length, _ := rdb.XLen(ctx, streamKey).Result()
	assert.Equal(t, int64(1), length)

	// XRANGE —— 读取消息（按 ID 范围）
	// - 表示最小 ID，+ 表示最大 ID
	messages, _ := rdb.XRange(ctx, streamKey, "-", "+").Result()
	assert.Len(t, messages, 1)
	assert.Equal(t, "user.registered", messages[0].Values["event"])

	rdb.Del(ctx, streamKey)
}

func TestStream_MaxLen(t *testing.T) {
	streamKey := testKey("stream:maxlen")

	// MAXLEN ~ 100：保持大约 100 条消息（~ 是近似，不精确）
	// MAXLEN 100：精确保持 100 条（但更慢）
	for i := range 150 {
		rdb.XAdd(ctx, &redis.XAddArgs{
			Stream: streamKey,
			MaxLen: 100,
			Approx: true, // ~ 近似模式，性能更好
			Values: map[string]string{
				seqKey: strconv.Itoa(i),
			},
		})
	}

	length, _ := rdb.XLen(ctx, streamKey).Result()
	// MAXLEN ~ 100 允许误差，实际可能在 95~150 之间
	assert.True(t, length >= 90 && length <= 150, "Stream length: %d", length)

	rdb.Del(ctx, streamKey)

	// 【坑】MAXLEN 只在 XADD 时删除旧消息，不会主动压缩已有的大量消息
	// 【坑】~ 近似模式下，实际保留的消息数可能比 MAXLEN 多
	// 【坑】MINID 参数可以按 ID 删除，比 MAXLEN 更适合时间维度的清理
}

func TestStream_ConsumerGroup(t *testing.T) {
	streamKey := testKey("stream:orders")
	groupName := "order-processors"
	consumer1 := "worker-1"
	consumer2 := "worker-2"

	// XGROUP CREATE MKSTREAM —— 创建消费者组（如果 Stream 不存在会自动创建）
	err := rdb.XGroupCreateMkStream(ctx, streamKey, groupName, "0").Err()
	require.NoError(t, err)

	// 生产者发送消息
	for i := 1; i <= 6; i++ {
		rdb.XAdd(ctx, &redis.XAddArgs{
			Stream: streamKey,
			Values: map[string]string{
				"order_id": fmt.Sprintf("ORD-%04d", i),
				"amount":   strconv.Itoa(i * 100),
			},
		})
	}

	// XREADGROUP —— 消费者读取消消息
	// ">" 表示只读取新消息（未被投递过的）
	msgs1, _ := rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    groupName,
		Consumer: consumer1,
		Streams:  []string{streamKey, ">"},
		Count:    3,
	}).Result()
	require.Len(t, msgs1, 1)
	require.Len(t, msgs1[0].Messages, 3) // worker-1 消费了 3 条

	msgs2, _ := rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    groupName,
		Consumer: consumer2,
		Streams:  []string{streamKey, ">"},
		Count:    3,
	}).Result()
	require.Len(t, msgs2, 1)
	require.Len(t, msgs2[0].Messages, 3) // worker-2 消费了 3 条

	// XACK —— 确认消息（从 PEL 中移除）
	for _, msg := range msgs1[0].Messages {
		rdb.XAck(ctx, streamKey, groupName, msg.ID)
	}
	for _, msg := range msgs2[0].Messages {
		rdb.XAck(ctx, streamKey, groupName, msg.ID)
	}

	// XPENDING —— 查看待确认消息（PEL）
	pending, _ := rdb.XPending(ctx, streamKey, groupName).Result()
	assert.Equal(t, int64(0), pending.Count) // 全部 ACK 了

	rdb.Del(ctx, streamKey)
	rdb.XGroupDestroy(ctx, streamKey, groupName)
}

func TestStream_PendingAndReclaim(t *testing.T) {
	streamKey := testKey("stream:pending")
	groupName := "test-group"
	consumer1 := "worker-1"

	rdb.XGroupCreateMkStream(ctx, streamKey, groupName, "0")

	// 生产 3 条消息
	for i := 1; i <= 3; i++ {
		rdb.XAdd(ctx, &redis.XAddArgs{
			Stream: streamKey,
			Values: map[string]string{seqKey: strconv.Itoa(i)},
		})
	}

	// worker-1 消费但不 ACK（模拟崩溃）
	msgs, err := rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    groupName,
		Consumer: consumer1,
		Streams:  []string{streamKey, ">"},
		Count:    3,
	}).Result()
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	require.Len(t, msgs[0].Messages, 3)

	// XPENDING —— 查看 PEL（消息在 PEL 中等待 ACK）
	pendingExt, _ := rdb.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: streamKey,
		Group:  groupName,
		Start:  "-",
		End:    "+",
		Count:  10,
	}).Result()
	assert.Len(t, pendingExt, 3) // 3 条消息在 PEL 中

	// XCLAIM —— 超时后，其他消费者可以认领（reclaim）
	// 超过 60000ms 未 ACK 的消息可以被认领
	claimed, _ := rdb.XClaim(ctx, &redis.XClaimArgs{
		Stream:   streamKey,
		Group:    groupName,
		Consumer: "worker-2", // worker-2 认领 worker-1 的消息
		MinIdle:  60 * time.Second,
		Messages: []string{msgs[0].Messages[0].ID},
	}).Result()
	// 如果 idle 时间不够，可能 claim 不到消息

	// XAUTOCLAIM —— 自动认领（Redis 6.2+）
	// 自动找到 idle 超时的消息并认领
	autoClaimed, _, err := rdb.XAutoClaim(ctx, &redis.XAutoClaimArgs{
		Stream:   streamKey,
		Group:    groupName,
		Consumer: "worker-2",
		MinIdle:  60 * time.Second,
		Start:    "0-0",
		Count:    10,
	}).Result()
	// 如果 idle 时间不够，返回空

	_ = claimed
	_ = autoClaimed
	_ = err

	// 确认所有消息
	for _, msg := range msgs[0].Messages {
		rdb.XAck(ctx, streamKey, groupName, msg.ID)
	}

	rdb.Del(ctx, streamKey)
	rdb.XGroupDestroy(ctx, streamKey, groupName)

	// 【坑】XCLAIM 需要指定消息 ID，不知道 ID 就无法 claim
	// 【坑】XAUTOCLAIM 比 XCLAIM 更好用，但需要 Redis 6.2+
	// 【坑】忘记 XACK 会导致 PEL 不断增长，占用内存
}

func TestStream_BackfillAndTrim(t *testing.T) {
	streamKey := testKey("stream:backfill")

	// 指定 ID 写入消息（模拟历史数据）
	rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: streamKey,
		ID:     "1688000001000-0", // 精确指定 ID
		Values: map[string]string{seqKey: "1"},
	})
	rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: streamKey,
		ID:     "1688000002000-0",
		Values: map[string]string{seqKey: "2"},
	})
	rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: streamKey,
		ID:     "1688000003000-0",
		Values: map[string]string{seqKey: "3"},
	})

	// XRANGE 读取指定范围
	msgs, _ := rdb.XRangeN(ctx, streamKey, "1688000001000-0", "1688000002000-0", 10).Result()
	assert.Len(t, msgs, 2)

	// XTRIM —— 修剪 Stream（保留最新 2 条）
	rdb.XTrimMaxLen(ctx, streamKey, 2)

	length, _ := rdb.XLen(ctx, streamKey).Result()
	assert.Equal(t, int64(2), length)

	// XREVRANGE —— 反向读取（最新在前）
	latest, _ := rdb.XRevRangeN(ctx, streamKey, "+", "-", 2).Result()
	assert.Len(t, latest, 2)
	assert.Equal(t, "3", latest[0].Values[seqKey]) // 最新的在前

	rdb.Del(ctx, streamKey)
}

func TestStream_RebalanceConsumer(t *testing.T) {
	// 演示消费者组的负载均衡
	streamKey := testKey("stream:rebalance")
	groupName := "balanced-group"
	consumers := []string{"worker-A", "worker-B", "worker-C"}

	rdb.XGroupCreateMkStream(ctx, streamKey, groupName, "0")

	// 生产 9 条消息
	for i := range 9 {
		rdb.XAdd(ctx, &redis.XAddArgs{
			Stream: streamKey,
			Values: map[string]string{
				"task_id": fmt.Sprintf("task-%d", i),
			},
		})
	}

	// 3 个消费者各自消费 3 条
	for _, consumer := range consumers {
		msgs, _ := rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    groupName,
			Consumer: consumer,
			Streams:  []string{streamKey, ">"},
			Count:    3,
		}).Result()
		assert.Len(t, msgs[0].Messages, 3, "%s should consume 3 messages", consumer)

		// 确认所有消息
		for _, msg := range msgs[0].Messages {
			rdb.XAck(ctx, streamKey, groupName, msg.ID)
		}
	}

	pending, _ := rdb.XPending(ctx, streamKey, groupName).Result()
	assert.Equal(t, int64(0), pending.Count)

	rdb.Del(ctx, streamKey)
	rdb.XGroupDestroy(ctx, streamKey, groupName)
}

func TestStream_WithErrorHandling(t *testing.T) {
	streamKey := testKey("stream:error")
	groupName := "error-group"
	consumer := "processor"

	rdb.XGroupCreateMkStream(ctx, streamKey, groupName, "0")

	// 生产 2 条消息
	rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: streamKey,
		Values: map[string]string{"type": "order"},
	})
	rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: streamKey,
		Values: map[string]string{"type": "payment"},
	})

	// 消费消息
	msgs, err := rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    groupName,
		Consumer: consumer,
		Streams:  []string{streamKey, ">"},
		Count:    2,
	}).Result()
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	require.Len(t, msgs[0].Messages, 2)

	// 模拟处理：第一条成功，第二条失败
	for _, msg := range msgs[0].Messages {
		eventType, _ := msg.Values["type"].(string)
		if eventType == "payment" {
			// 处理失败，不 ACK（消息留在 PEL 中，下次重试）
			t.Logf("消息 %s 处理失败，保留在 PEL 中", msg.ID)
			continue
		}
		// 处理成功，ACK
		rdb.XAck(ctx, streamKey, groupName, msg.ID)
	}

	// 检查 PEL
	pendingExt, _ := rdb.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: streamKey,
		Group:  groupName,
		Start:  "-",
		End:    "+",
		Count:  10,
	}).Result()
	assert.Len(t, pendingExt, 1) // 还有 1 条未 ACK

	rdb.Del(ctx, streamKey)
	rdb.XGroupDestroy(ctx, streamKey, groupName)
}

func TestStream_BlockedConsumer(t *testing.T) {
	streamKey := testKey("stream:blocked")
	groupName := "blocked-group"
	consumer := "block-worker"

	rdb.XGroupCreateMkStream(ctx, streamKey, groupName, "0")

	type receivedMsg struct {
		values map[string]string
	}

	var received []receivedMsg
	var mu sync.Mutex

	// 模拟阻塞消费者
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		msgs, err := rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    groupName,
			Consumer: consumer,
			Streams:  []string{streamKey, ">"},
			Count:    1,
			Block:    1 * time.Second,
		}).Result()
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, redis.Nil) {
				return
			}
			return
		}
		for _, stream := range msgs {
			for _, msg := range stream.Messages {
				strValues := make(map[string]string, len(msg.Values))
				for k, v := range msg.Values {
					strValues[k] = fmt.Sprintf("%v", v)
				}
				mu.Lock()
				received = append(received, receivedMsg{values: strValues})
				mu.Unlock()
				rdb.XAck(ctx, streamKey, groupName, msg.ID)
			}
		}
	}()

	// 等一下让消费者进入阻塞状态，再发送消息
	time.Sleep(500 * time.Millisecond)
	rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: streamKey,
		Values: map[string]string{"type": "notification"},
	})

	<-done

	mu.Lock()
	assert.Len(t, received, 1)
	if len(received) > 0 {
		assert.Equal(t, "notification", received[0].values["type"])
	}
	mu.Unlock()

	rdb.Del(ctx, streamKey)
	rdb.XGroupDestroy(ctx, streamKey, groupName)
}

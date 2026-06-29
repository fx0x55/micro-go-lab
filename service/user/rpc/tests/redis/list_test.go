//go:build redis

package redis_test

import (
	"fmt"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// List 列表 — 双端链表 + 压缩列表
// ============================================================================
//
// 【为什么用 List】
// Redis List 是双端链表（quicklist，底层 listpack + linkedlist 混合，Redis 7+ 重命名自 ziplist），支持 O(1) 的两端插入和弹出。
// 天然适合队列（LPUSH + RPOP）和栈（LPUSH + LPOP），还可以做时间线、最近浏览记录等。
//
// 【适用场景】
//   - 消息队列（简单场景）：LPUSH 生产 + RPOP/BRPOP 消费
//   - 最近 N 条记录：LPUSH + LTRIM（保留最新 N 条）
//   - 栈：LPUSH + LPOP（后进先出）
//   - 任务列表：LPUSH + RPOP/LPOP（FIFO 队列）
//   - 评论/弹幕列表：LPUSH（最新在前）+ LRANGE 取最近
//   - 有界缓冲区：LPUSH + LTRIM 控制大小
//
// 【坑和注意事项】
//   1. List 没有按值删除（LREM 有，但 O(N) 且只能删值不能删索引）
//   2. LINDEX 是 O(N)（不是 O(1)！），不要当数组用
//   3. List 不支持按 rank 查询，需要遍历（LRANGE 0 -1 会返回全部元素）
//   4. 大 List（>10000 元素）的 LTRIM/LRANGE 会阻塞 Redis
//   5. LPUSH + RPOP 组合在消费者崩溃时会丢消息（没有 ACK 机制，考虑用 Stream）
//   6. BRPOP 是阻塞弹出，超时返回空，客户端需要处理轮询逻辑
//   7. List 没有 TTL（整个 key 有，元素没有），用 LTRIM 做有界列表

func TestList_BasicQueue(t *testing.T) {
	key := testKey("list:queue")

	// LPUSH —— 左端插入（队列尾部 / 栈顶）
	rdb.LPush(ctx, key, "task1", "task2")
	// RPOP —— 右端弹出（队列头部 / 栈底）
	// FIFO: LPUSH + RPOP
	// LIFO: LPUSH + LPOP

	val, err := rdb.RPop(ctx, key).Result()
	require.NoError(t, err)
	assert.Equal(t, "task1", val) // LPUSH 先插入的先出来（FIFO）

	val, _ = rdb.RPop(ctx, key).Result()
	assert.Equal(t, "task2", val)

	// List 为空后 RPOP 返回 redis.Nil
	_, err = rdb.RPop(ctx, key).Result()
	require.ErrorIs(t, err, redis.Nil)

	rdb.Del(ctx, key)
}

func TestList_BasicStack(t *testing.T) {
	key := testKey("list:stack")

	// LPUSH + LPOP = LIFO（后进先出）= 栈
	rdb.LPush(ctx, key, "page1", "page2", "page3")

	val, _ := rdb.LPop(ctx, key).Result()
	assert.Equal(t, "page3", val) // 最后插入的先出来

	val, _ = rdb.LPop(ctx, key).Result()
	assert.Equal(t, "page2", val)

	rdb.Del(ctx, key)
}

func TestList_BoundedList(t *testing.T) {
	key := testKey("list:recent")

	// LPUSH + LTRIM = 有界列表（只保留最近 N 条）
	// 典型场景：最近 5 条浏览记录
	for i := 1; i <= 10; i++ {
		rdb.LPush(ctx, key, fmt.Sprintf("item:%d", i))
	}
	rdb.LTrim(ctx, key, 0, 4) // 保留索引 0~4（最新的 5 个）

	length, _ := rdb.LLen(ctx, key).Result()
	assert.Equal(t, int64(5), length)

	// LRANGE 0 -1 获取全部元素
	items, _ := rdb.LRange(ctx, key, 0, -1).Result()
	assert.Len(t, items, 5)
	// LPUSH 的顺序：最新在左（索引 0）
	assert.Equal(t, "item:10", items[0])
	assert.Equal(t, "item:6", items[4])

	rdb.Del(ctx, key)
}

func TestList_BlockedPop(t *testing.T) {
	key := testKey("list:blocked")

	// BRPOP —— 阻塞弹出，直到有元素或超时
	// 常用于简单消息队列的消费者端

	// 先推入一条消息
	rdb.LPush(ctx, key, "msg:hello")

	// BRPOP 有元素时立即返回
	vals, err := rdb.BRPop(ctx, 0, key).Result()
	require.NoError(t, err)
	assert.Equal(t, key, vals[0]) // 返回 [key, value]
	assert.Equal(t, "msg:hello", vals[1])

	// BRPOP 无元素时会阻塞（这里用短超时演示）
	_, err = rdb.BRPop(ctx, 100*1000000, key).Result() // 100ms
	// 超时返回 redis.Nil
	if err != nil {
		require.ErrorIs(t, err, redis.Nil)
	}

	rdb.Del(ctx, key)

	// 【坑】BRPOP 在多个 key 时只弹出第一个有数据的 key 的元素
	// 【坑】超时参数单位是时间.Duration（纳秒），不是秒
	// 【坑】消费者进程崩溃后，BRPOP 弹出但未处理的消息会丢失
}

func TestList_SearchAndDelete(t *testing.T) {
	key := testKey("list:lrem")

	rdb.LPush(ctx, key, "apple", "banana", "apple", "cherry", "apple")
	// LPush 后列表顺序（后插入的在头部）: ["apple", "cherry", "apple", "banana", "apple"]

	// LREM count>0: 从头到尾删除 count 个值为 value 的元素
	// LREM count=0: 删除所有值为 value 的元素
	// LREM count<0: 从尾到头删除 |count| 个值为 value 的元素
	deleted, _ := rdb.LRem(ctx, key, 2, "apple").Result()
	assert.Equal(t, int64(2), deleted) // 删了 2 个 apple（从头部开始删）

	items, _ := rdb.LRange(ctx, key, 0, -1).Result()
	assert.Equal(t, []string{"cherry", "banana", "apple"}, items) // 还剩 1 个 apple

	rdb.Del(ctx, key)
}

func TestList_IndexAndRange(t *testing.T) {
	key := testKey("list:index")

	rdb.RPush(ctx, key, "a", "b", "c", "d", "e")

	// LINDEX O(N) —— 按索引获取（不要频繁使用！）
	val, _ := rdb.LIndex(ctx, key, 2).Result()
	assert.Equal(t, "c", val)

	// 负索引从右开始：-1 是最后一个
	val, _ = rdb.LIndex(ctx, key, -1).Result()
	assert.Equal(t, "e", val)

	// LRANGE 0 -1 返回全部元素
	// 负索引：-1 是最后一个，-2 是倒数第二个
	vals, _ := rdb.LRange(ctx, key, 1, 3).Result()
	assert.Equal(t, []string{"b", "c", "d"}, vals)

	// 【坑】LRANGE 0 -1 返回全部元素，元素多时 O(N) 会阻塞
	// 【坑】LINDEX 对大列表是 O(N)，不要循环 LINDEX 遍历

	rdb.Del(ctx, key)
}

func TestList_InsertOperations(t *testing.T) {
	key := testKey("list:insert")

	rdb.RPush(ctx, key, "a", "c", "e")

	// LINSERT BEFORE/AFTER —— 在指定值前/后插入
	// O(N) 查找时间！
	rdb.LInsertBefore(ctx, key, "c", "B") // 在 "c" 前面插入 "B"
	rdb.LInsertAfter(ctx, key, "c", "D")  // 在 "c" 后面插入 "D"

	items, _ := rdb.LRange(ctx, key, 0, -1).Result()
	assert.Equal(t, []string{"a", "B", "c", "D", "e"}, items)

	// LINSERT 找不到目标值时返回 -1
	deleted, _ := rdb.LInsertBefore(ctx, key, "z", "X").Result()
	assert.Equal(t, int64(-1), deleted)

	rdb.Del(ctx, key)

	// 【坑】LINSERT 是 O(N) 操作，列表越长越慢
	// 【坑】LINSERT 只插入第一个匹配的值
}

func TestList_MoveBetweenLists(t *testing.T) {
	srcKey := testKey("list:move:src")
	dstKey := testKey("list:move:dst")

	rdb.RPush(ctx, srcKey, "a", "b", "c")

	// RPOPLPUSH / LMOVE —— 原子地从一个 List 弹出并推入另一个
	// 常用于可靠队列：从待处理队列取出，推入处理中队列
	val, err := rdb.LMove(ctx, srcKey, dstKey, "RIGHT", "LEFT").Result()
	require.NoError(t, err)
	assert.Equal(t, "c", val) // 从 src 右端弹出（FIFO 顺序取最早的）

	items, _ := rdb.LRange(ctx, dstKey, 0, -1).Result()
	assert.Equal(t, []string{"c"}, items)

	srcItems, _ := rdb.LRange(ctx, srcKey, 0, -1).Result()
	assert.Equal(t, []string{"a", "b"}, srcItems)

	rdb.Del(ctx, srcKey, dstKey)

	// 【坑】LMOVE 是原子操作，适合做可靠队列
	// 【坑】可靠队列仍需手动 ACK（处理完从处理中队列删除）
	// 【坑】大量元素的 LMOVE 不会导致问题，但要注意消费者处理速度
}

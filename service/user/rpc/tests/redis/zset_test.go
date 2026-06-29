//go:build redis

package redis_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Sorted Set (ZSet) 有序集合 — 带分数的集合，天然有序
// ============================================================================
//
// 【为什么用 ZSet】
// ZSet 每个元素关联一个 float64 分数(score)，按 score 自动排序。
// 支持 O(logN) 的插入/删除/查询，以及范围查询（ZRANGEBYSCORE、ZRANK）。
// Redis 中排行榜、延时队列、时间线等场景的首选数据结构。
//
// 【适用场景】
//   - 排行榜/积分榜：ZADD game:score player score → ZREVRANGE 取 Top N
//   - 延时队列：score = 执行时间戳，ZRANGEBYSCORE 取到期任务
//   - 带权重的优先级队列：score 越大优先级越高
//   - 时间线/Feed：score = 时间戳，ZREVRANGE 取最新
//   - 滑动窗口限流：score = 时间戳，ZREMRANGEBYSCORE 清理过期
//   - 范围查找：按价格区间查询商品（ZADD 商品ID 价格 → ZRANGEBYSCORE）
//
// 【坑和注意事项】
//   1. score 是 float64，精度有限（52位有效数字），大时间戳可能有精度损失
//   2. 相同 score 的元素按字典序排序，不是插入顺序
//   3. ZRANGEBYSCORE 在数据量大时是 O(logN + M)，M 是返回元素数
//   4. 大 ZSet（>10000）的 ZRANGE 会返回大量数据，用 LIMIT 分页
//   5. ZUNIONSTORE / ZINTERSTORE 时间复杂度高，不要频繁对大 ZSet 做
//   6. ZSet 不支持按索引插入（只能按 score 排序），不是真正的链表
//   7. score 相同时，元素按 member 字典序排序（不是插入顺序！）

func TestZSet_BasicOps(t *testing.T) {
	key := testKey("zset:leaderboard")

	// ZADD —— 添加元素（score:member）
	added, err := rdb.ZAdd(ctx, key,
		redis.Z{Score: 100, Member: "Alice"},
		redis.Z{Score: 250, Member: "Bob"},
		redis.Z{Score: 180, Member: "Charlie"},
	).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(3), added)

	// ZRANGE —— 按 score 升序获取
	ranked, _ := rdb.ZRangeWithScores(ctx, key, 0, -1).Result()
	assert.Equal(t, "Alice", ranked[0].Member)   // 100
	assert.Equal(t, "Charlie", ranked[1].Member) // 180
	assert.Equal(t, "Bob", ranked[2].Member)     // 250

	// ZREVRANGE —— 按 score 降序（排行榜常用）
	top2, _ := rdb.ZRevRangeWithScores(ctx, key, 0, 1).Result()
	assert.Equal(t, "Bob", top2[0].Member)     // 250
	assert.Equal(t, "Charlie", top2[1].Member) // 180

	// ZCARD —— 元素总数
	count, _ := rdb.ZCard(ctx, key).Result()
	assert.Equal(t, int64(3), count)

	rdb.Del(ctx, key)
}

func TestZSet_UpdateScore(t *testing.T) {
	key := testKey("zset:update")

	rdb.ZAdd(ctx, key, redis.Z{Score: 100, Member: "player1"})

	// ZADD 对已存在的 member 会更新 score（不是追加）
	newAdded, _ := rdb.ZAdd(ctx, key, redis.Z{Score: 300, Member: "player1"}).Result()
	assert.Equal(t, int64(0), newAdded) // 返回 0 表示更新而非新增

	score, _ := rdb.ZScore(ctx, key, "player1").Result()
	assert.InEpsilon(t, 300, score, 0)

	// ZINCRBY —— 原子地增加 score
	rdb.ZIncrBy(ctx, key, 50, "player1")
	score, _ = rdb.ZScore(ctx, key, "player1").Result()
	assert.InEpsilon(t, 350, score, 0)

	rdb.Del(ctx, key)
}

func TestZSet_RankAndScore(t *testing.T) {
	key := testKey("zset:rank")

	rdb.ZAdd(ctx, key,
		redis.Z{Score: 100, Member: "Alice"},
		redis.Z{Score: 200, Member: "Bob"},
		redis.Z{Score: 300, Member: "Charlie"},
	)

	// ZRANK —— 获取排名（从 0 开始，score 越低排名越前）
	rank, _ := rdb.ZRank(ctx, key, "Alice").Result()
	assert.Equal(t, int64(0), rank) // 第 1 名（索引 0）

	// ZREVRANK —— 降序排名（排行榜：分数越高排名越前）
	rank, _ = rdb.ZRevRank(ctx, key, "Alice").Result()
	assert.Equal(t, int64(2), rank) // 倒数第 1

	// ZSCORE —— 获取分数
	score, _ := rdb.ZScore(ctx, key, "Bob").Result()
	assert.InEpsilon(t, 200, score, 0)

	// ZMSCORE —— 批量获取分数（Redis 6.2+）
	scores, _ := rdb.ZMScore(ctx, key, "Alice", "Bob", "NotExist").Result()
	assert.InEpsilon(t, 100, scores[0], 0)
	assert.InEpsilon(t, 200, scores[1], 0)
	assert.InDelta(t, 0, scores[2], 0.001) // 不存在的成员返回 0

	rdb.Del(ctx, key)
}

func TestZSet_RangeQueries(t *testing.T) {
	key := testKey("zset:range")

	// 场景：商品价格排序
	rdb.ZAdd(ctx, key,
		redis.Z{Score: 29.99, Member: "product:1001"},
		redis.Z{Score: 99.99, Member: "product:1002"},
		redis.Z{Score: 199.99, Member: "product:1003"},
		redis.Z{Score: 59.99, Member: "product:1004"},
		redis.Z{Score: 149.99, Member: "product:1005"},
	)

	results, err := rdb.ZRangeArgs(ctx, redis.ZRangeArgs{
		Key:     key,
		Start:   50.0,
		Stop:    150.0,
		ByScore: true,
	}).Result()
	require.NoError(t, err)
	assert.Len(t, results, 3) // 59.99, 99.99, 149.99

	// 带 LIMIT 分页
	results, _ = rdb.ZRangeArgs(ctx, redis.ZRangeArgs{
		Key:     key,
		Start:   "-inf",
		Stop:    "+inf",
		Offset:  0,
		Count:   2,
		ByScore: true,
	}).Result()
	assert.Len(t, results, 2) // 最便宜的 2 个

	// ZRANGEBYSCORE with WITHSCORES
	resultsWithScores, _ := rdb.ZRangeByScoreWithScores(ctx, key, &redis.ZRangeBy{
		Min: "50",
		Max: "150",
	}).Result()
	assert.Len(t, resultsWithScores, 3)
	assert.Equal(t, "product:1004", resultsWithScores[0].Member) // 59.99

	rdb.Del(ctx, key)

	// 【坑】ZRANGEBYSCORE 用的是闭区间 [min, max]
	// 用 ( 表示开区间：ZRANGEBYSCORE key (50 (150
	// 用 -inf/+inf 表示无穷
}

func TestZSet_DelayedQueue(t *testing.T) {
	key := testKey("zset:delay_queue")

	// 场景：延时队列 —— score 是执行时间戳
	now := time.Now().UnixMilli()
	task1Time := now + 5000  // 5 秒后
	task2Time := now + 10000 // 10 秒后
	task3Time := now + 3000  // 3 秒后

	rdb.ZAdd(ctx, key,
		redis.Z{Score: float64(task1Time), Member: "task:email:1001"},
		redis.Z{Score: float64(task2Time), Member: "task:sms:2002"},
		redis.Z{Score: float64(task3Time), Member: "task:push:3003"},
	)

	// 取到期任务（score < 当前时间）
	expired, _ := rdb.ZRangeArgs(ctx, redis.ZRangeArgs{
		Key:     key,
		Start:   "-inf",
		Stop:    "+inf",
		Count:   1,
		ByScore: true,
	}).Result()
	assert.Len(t, expired, 1) // 最早到期的任务

	// ZPOPMIN —— 弹出最小 score 的元素（原子操作，取出并删除）
	result, _ := rdb.ZPopMin(ctx, key, 1).Result()
	assert.Len(t, result, 1)

	rdb.Del(ctx, key)

	// 【坑】ZPOPMIN 不是原子的"取出-处理-删除"，消费者崩溃会丢消息
	// 【坑】生产环境建议用 Lua 脚本实现原子的"取出并处理"
}

func TestZSet_SlidingWindowRateLimit(t *testing.T) {
	key := testKey("zset:rate_limit")
	userID := "user:1001"
	rlKey := key + ":" + userID

	// 滑动窗口限流：1 分钟内最多 5 次请求
	window := int64(60) // 秒
	limit := int64(5)

	now := time.Now().UnixMilli()

	// 1. 移除窗口外的记录
	minScore := strconv.FormatInt(now-window*1000, 10)
	rdb.ZRemRangeByScore(ctx, rlKey, "0", minScore)

	// 2. 添加当前请求
	rdb.ZAdd(ctx, rlKey, redis.Z{Score: float64(now), Member: now})

	// 3. 统计窗口内请求数
	count, _ := rdb.ZCard(ctx, rlKey).Result()

	if count > limit {
		// 超过限制，删除刚添加的记录
		rdb.ZRem(ctx, rlKey, now)
		t.Log("请求被限流")
	} else {
		t.Log("请求通过")
	}

	// 设置 key 过期时间（兜底清理）
	rdb.Expire(ctx, rlKey, time.Duration(window)*time.Second)

	rdb.Del(ctx, rlKey)

	// 【坑】滑动窗口限流的 ZSet 会不断增长，必须配合 EXPIRE 或定期清理
	// 【坑】并发场景下 ZREMRANGEBYSCORE + ZADD 不是原子操作，高并发可能漏限流
	// 【坑】更精确的限流用 Lua 脚本保证原子性
}

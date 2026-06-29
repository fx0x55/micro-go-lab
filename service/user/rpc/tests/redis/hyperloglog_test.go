//go:build redis

package redis_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// HyperLogLog 基数计数器 — 概率性数据结构，极省内存的去重计数
// ============================================================================
//
// 【为什么用 HyperLogLog】
// HyperLogLog 用固定 12KB 内存估算任意规模集合的基数（去重后的元素数量）。
// 误差率约 0.81%，百万级元素也只需 12KB——比 Set 省几个数量级的内存。
// 适合"大概有多少"这种不需要精确计数的场景。
//
// 【适用场景】
//   - UV 统计：统计某页面/广告的独立访客数（不需要精确数字）
//   - 搜索词统计：统计某天有多少不同的搜索词
//   - IP 去重：统计某时段有多少独立 IP 访问
//   - 设备指纹统计：统计有多少独立设备
//   - 实时热点检测：结合时间窗口统计独立 key 数
//
// 【坑和注意事项】
//   1. 误差率 0.81%：100 万 UV 实际可能显示 99.19 万或 100.81 万
//   2. 不支持获取元素列表（只能计数，不能知道具体有哪些元素）
//   3. 不支持删除（PFADD 只能添加，不能 PFRM）
//   4. PFMERGE 合并多个 HLL 结果时，每个 HLL 都需要保留原始 key
//   5. 小数据量（<100 元素）时精度较差，大数据量（>10000）时误差稳定
//   6. 每个 HLL 固定占 12KB，即使只存 1 个元素
//   7. 不支持事务，PFADD + PFCOUNT 不是原子操作

func TestHyperLogLog_BasicOps(t *testing.T) {
	key := testKey("hll:uv:2026-06-28")

	// PFADD —— 添加元素（自动去重）
	added, err := rdb.PFAdd(ctx, key, "user:1001", "user:1002", "user:1003").Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), added) // 1 表示 HLL 发生了变化

	// 重复添加不会增加计数
	added, _ = rdb.PFAdd(ctx, key, "user:1001", "user:1004").Result()
	assert.Equal(t, int64(1), added) // 有新元素 user:1004

	// PFCOUNT —— 基数估算
	count, _ := rdb.PFCount(ctx, key).Result()
	assert.Equal(t, int64(4), count) // user:1001, 1002, 1003, 1004

	rdb.Del(ctx, key)
}

func TestHyperLogLog_LargeDataset(t *testing.T) {
	key := testKey("hll:large")

	// 插入 10000 个不同的元素
	args := make([]any, 10000)
	for i := range args {
		args[i] = fmt.Sprintf("item:%d", i)
	}

	err := rdb.PFAdd(ctx, key, args...).Err()
	require.NoError(t, err)

	// PFCOUNT 估算值允许 2% 误差（HLL 标准误差 ~0.81%，放宽到 2% 避免 flaky）
	count, _ := rdb.PFCount(ctx, key).Result()
	t.Logf("实际插入: 10000, PFCOUNT 估算: %d", count)
	assert.InDelta(t, 10000, count, 200)

	// 再插入 5000 个新元素 + 一些重复的
	newArgs := make([]any, 5000)
	for i := range newArgs {
		newArgs[i] = fmt.Sprintf("item:%d", i+10000) // 10000~14999
	}
	rdb.PFAdd(ctx, key, newArgs...)

	count, _ = rdb.PFCount(ctx, key).Result()
	t.Logf("实际总数: 15000, PFCOUNT 估算: %d", count)
	assert.InDelta(t, 15000, count, 300) // 2% of 15000

	rdb.Del(ctx, key)

	// 【坑】大量 PFADD 时性能会下降，因为 HLL 需要计算多个哈希值
	// 【坑】对于小数据集（<100），用 Set + SCARD 更精确
}

func TestHyperLogLog_Merge(t *testing.T) {
	// 场景：合并多个时间窗口的 UV 统计
	hour1Key := testKey("hll:uv:2026-06-28:10")
	hour2Key := testKey("hll:uv:2026-06-28:11")
	hour3Key := testKey("hll:uv:2026-06-28:12")

	// 每小时的 UV
	rdb.PFAdd(ctx, hour1Key, "user:1", "user:2", "user:3")
	rdb.PFAdd(ctx, hour2Key, "user:2", "user:4", "user:5")
	rdb.PFAdd(ctx, hour3Key, "user:3", "user:5", "user:6")

	// PFMERGE —— 合并多个 HLL
	dayKey := testKey("hll:uv:2026-06-28")
	rdb.PFMerge(ctx, dayKey, hour1Key, hour2Key, hour3Key)

	// PFCOUNT 传多个 key 也可以（等同于 PFMERGE 后 PFCOUNT）
	count, _ := rdb.PFCount(ctx, hour1Key, hour2Key, hour3Key).Result()
	assert.Equal(t, int64(6), count) // user:1~6，去重后 6 个

	dayCount, _ := rdb.PFCount(ctx, dayKey).Result()
	assert.Equal(t, int64(6), dayCount)

	rdb.Del(ctx, hour1Key, hour2Key, hour3Key, dayKey)

	// 【坑】PFMERGE 会覆盖目标 key 的原值
	// 【坑】合并后的 HLL 不能拆分回原始的小时维度数据
}

func TestHyperLogLog_NoDelete(t *testing.T) {
	key := testKey("hll:nodelete")

	rdb.PFAdd(ctx, key, "a", "b", "c")

	count, _ := rdb.PFCount(ctx, key).Result()
	assert.Equal(t, int64(3), count)

	// HyperLogLog 不支持删除元素！
	// 想要"删除后重新统计"只能删除整个 key 重建
	rdb.Del(ctx, key)
	rdb.PFAdd(ctx, key, "a", "b") // 重建，只包含 a, b

	count, _ = rdb.PFCount(ctx, key).Result()
	assert.Equal(t, int64(2), count)

	rdb.Del(ctx, key)

	// 【坑】这是 HyperLogLog 最大的限制——不能删除单个元素
	// 【坑】需要删除+重新统计的场景，考虑用 Set + SCARD（但内存消耗大得多）
}

func TestHyperLogLog_UVComparison(t *testing.T) {
	// 对比 HyperLogLog vs Set 在 UV 统计中的内存消耗
	hllKey := testKey("hll:uv:compare")
	setKey := testKey("set:uv:compare")

	// 插入 100000 个"用户"
	n := 100000
	args := make([]any, n)
	members := make([]string, n)
	for i := range args {
		members[i] = fmt.Sprintf("user:%d", i)
		args[i] = members[i]
	}

	rdb.PFAdd(ctx, hllKey, args...)
	rdb.SAdd(ctx, setKey, args...)

	// 比较内存占用
	hllMem, _ := rdb.MemoryUsage(ctx, hllKey).Result()
	setMem, _ := rdb.MemoryUsage(ctx, setKey).Result()

	t.Logf("HyperLogLog 内存: %d bytes", hllMem)
	t.Logf("Set 内存: %d bytes", setMem)
	t.Logf("内存比: Set 是 HLL 的 %.1f 倍", float64(setMem)/float64(hllMem))

	// HLL 应该远小于 Set（HLL 固定 12KB）
	assert.Less(t, hllMem, setMem)

	// 精度对比
	hllCount, _ := rdb.PFCount(ctx, hllKey).Result()
	setCount, _ := rdb.SCard(ctx, setKey).Result()
	t.Logf("HLL 估算: %d, Set 精确: %d, 误差: %.2f%%",
		hllCount, setCount, float64(hllCount-setCount)/float64(setCount)*100)

	rdb.Del(ctx, hllKey, setKey)
}

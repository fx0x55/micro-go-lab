//go:build redis

package redis_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Set 集合 — 无序、不重复的字符串集合
// ============================================================================
//
// 【为什么用 Set】
// Set 是无序、元素唯一的集合，天然去重。
// 支持交集、并集、差集等集合运算，可以高效做"共同好友"、"标签过滤"、"抽奖去重"等。
//
// 【适用场景】
//   - 标签系统：article:1:tags -> {golang, redis, microservice}
//   - 共同好友：SINTER user:1:friends user:2:friends
//   - 抽奖/随机获取：SRANDMEMBER（不放回抽样）/ SPOP（放回抽样）
//   - 黑名单/白名单：SISMEMBER O(1) 判断是否存在
//   - 去重计数：SCARD 统计 UV（Unique Visitors）
//   - 好友关系：互关（SINTER）、我关注但没互关（SDIFF）
//
// 【坑和注意事项】
//   1. Set 元素是字符串，不是数字；"1" 和 1 是不同的元素
//   2. SINTER/SDIFF/SUNION 结果会创建新 key（如果结果大，注意内存）
//   3. Set 不支持按索引获取元素，只能随机获取（SRANDMEMBER）
//   4. 纯整数元素且数量 ≤512 时用 intset 编码；小集合（≤128 元素）用 listpack 编码（Redis 7+）；超过后转 hashtable
//   5. SMEMBERS 在大 Set 上是 O(N)，用 SSCAN 代替
//   6. Set 没有 TTL（整个 key 有，元素没有）
//   7. 大 Set 的 SINTER/SDIFF 可能很慢，考虑用 SINTERSTORE 异步计算

func TestSet_BasicOps(t *testing.T) {
	key := testKey("set:tags")

	// SADD —— 添加元素（自动去重）
	added, err := rdb.SAdd(ctx, key, "golang", "redis", "microservice", "golang").Result()
	require.NoError(t, err)
	assert.Equal(t, int64(3), added) // 只添加了 3 个（golang 重复了）

	// SMEMBERS —— 获取所有元素（无序！顺序可能和插入顺序不同）
	members, _ := rdb.SMembers(ctx, key).Result()
	assert.Len(t, members, 3)
	assert.Contains(t, members, "golang")
	assert.Contains(t, members, "redis")
	assert.Contains(t, members, "microservice")

	// SISMEMBER —— 判断是否是成员（O(1)）
	exists, _ := rdb.SIsMember(ctx, key, "golang").Result()
	assert.True(t, exists)

	exists, _ = rdb.SIsMember(ctx, key, "python").Result()
	assert.False(t, exists)

	// SCARD —— 元素个数
	count, _ := rdb.SCard(ctx, key).Result()
	assert.Equal(t, int64(3), count)

	// SREM —— 删除元素
	removed, _ := rdb.SRem(ctx, key, "redis").Result()
	assert.Equal(t, int64(1), removed)

	rdb.Del(ctx, key)
}

func TestSet_RandomMember(t *testing.T) {
	key := testKey("set:lottery")

	rdb.SAdd(ctx, key, "Alice", "Bob", "Charlie", "David", "Eve")

	// SRANDMEMBER —— 随机获取元素（不删除，可重复）
	winners := rdb.SRandMemberN(ctx, key, 2).Val()
	assert.Len(t, winners, 2)
	// 每次运行结果可能不同

	// SPOP —— 随机弹出元素（删除，不重复）
	// 适合抽奖场景
	popped := rdb.SPopN(ctx, key, 1).Val()
	assert.Len(t, popped, 1)

	count, _ := rdb.SCard(ctx, key).Result()
	assert.Equal(t, int64(4), count) // 弹出了 1 个

	rdb.Del(ctx, key)

	// 【坑】SRANDMEMBER count > 集合大小时，会返回所有元素（但不保证顺序）
	// 【坑】SPOP count > 集合大小时，只会返回全部元素
}

func TestSet_SetOperations(t *testing.T) {
	setA := testKey("set:A")
	setB := testKey("set:B")

	rdb.SAdd(ctx, setA, "golang", "python", "java", "c++")
	rdb.SAdd(ctx, setB, "python", "rust", "java", "go")

	// SINTER —— 交集（两者都有的元素）
	// 应用：共同好友、共同标签
	inter, _ := rdb.SInter(ctx, setA, setB).Result()
	assert.ElementsMatch(t, []string{"python", "java"}, inter)

	// SUNION —— 并集（所有元素）
	// 应用：合并标签、合并关注
	union, _ := rdb.SUnion(ctx, setA, setB).Result()
	assert.Len(t, union, 6)

	// SDIFF —— 差集（A 中有但 B 中没有的）
	// 应用：A 关注但 B 没关注的人
	diff, _ := rdb.SDiff(ctx, setA, setB).Result()
	assert.ElementsMatch(t, []string{"golang", "c++"}, diff)

	// SINTERSTORE / SUNIONSTORE / SDIFFSTORE —— 将结果存储到新 key
	storeKey := testKey("set:inter_store")
	rdb.SInterStore(ctx, storeKey, setA, setB)
	stored, _ := rdb.SMembers(ctx, storeKey).Result()
	assert.ElementsMatch(t, []string{"python", "java"}, stored)

	rdb.Del(ctx, setA, setB, storeKey)

	// 【坑】SINTER/SDIFF 的时间复杂度是 O(N*M)，N 和 M 是各集合的大小
	// 【坑】大集合的集合运算会阻塞 Redis，建议用 SINTERSTORE 异步计算
}

func TestSet_MembershipTracking(t *testing.T) {
	// 场景：用户签到打卡（30天一个 Set）
	userID := "user:1001"
	month := "2026-06"
	key := testKey("set:checkin:" + userID + ":" + month)

	// SISMEMBER —— O(1) 判断今天是否已签到
	exists, _ := rdb.SIsMember(ctx, key, "06-28").Result()
	assert.False(t, exists) // 今天还没签到

	// SADD —— 签到（自动去重，重复签到不会增加计数）
	added, _ := rdb.SAdd(ctx, key, "06-28").Result()
	assert.Equal(t, int64(1), added)

	exists, _ = rdb.SIsMember(ctx, key, "06-28").Result()
	assert.True(t, exists)

	// SCARD —— 本月签到天数
	days, _ := rdb.SCard(ctx, key).Result()
	assert.Equal(t, int64(1), days)

	// 连续签到天数（需要额外逻辑）
	// 【坑】Set 无法高效查询"最近 N 天连续签到"，需要用位图(Bitmap)或有序集合(Sorted Set)

	rdb.Del(ctx, key)
}

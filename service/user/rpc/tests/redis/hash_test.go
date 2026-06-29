//go:build redis

package redis_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Hash 哈希 — 结构化对象存储
// ============================================================================
//
// 【为什么用 Hash】
// Hash 把一组字段名-值存在一个 key 下，比用多个 String key 存储更省内存。
// Redis 的 Hash 在字段数少（≤512）且值短（≤64字节）时使用 listpack 编码（Redis 7+ 重命名自 ziplist），
// 比等量 String key 的 SDS 编码节省大量内存（没有额外的 key 开销）。
//
// 【适用场景】
//   - 用户信息存储：user:1001 -> {name, email, avatar, ...}
//   - 购物车：cart:user1001 -> {sku1001: 2, sku2005: 1}
//   - 配置中心：config:app -> {db_host, cache_ttl, ...}
//   - 对象的部分更新：只改 name 不动其他字段（比序列化整个 JSON 再写回去高效）
//   - 表格/表单数据：每行一个 hash key，列名 = 字段名
//
// 【坑和注意事项】
//   1. 每个 field 都有开销，字段数太多（>512）性能下降（listpack 转 hashtable）
//   2. HGETALL 会返回所有字段，字段数很多时 O(N) 可能慢，用 HSCAN 或指定字段
//   3. Hash 没有 TTL（只能对整个 key 设置 TTL，不能对单个 field 设置）
//   4. field 的值是字符串，不能直接做数学运算（不能 HINCRBY 浮点数）
//   5. 大量 field 的 Hash 建议拆分（比如 user:1001:profile 和 user:1001:settings）
//   6. 不要用 field 存超大值（>1MB），这时用单独的 String key 更合适

func TestHash_BasicOps(t *testing.T) {
	key := testKey("hash:user:1001")

	// HSET —— 设置单个字段
	rdb.HSet(ctx, key, "name", "张三")

	// HMSET —— 批量设置多个字段（也可直接传 map）
	err := rdb.HMSet(ctx, key,
		"email", "zhangsan@example.com",
		"age", "28",
		"city", "北京",
	).Err()
	require.NoError(t, err)

	// HGET —— 获取单个字段
	name, err := rdb.HGet(ctx, key, "name").Result()
	require.NoError(t, err)
	assert.Equal(t, "张三", name)

	// HMGET —— 批量获取多个字段
	values, err := rdb.HMGet(ctx, key, "name", "email", "phone").Result()
	require.NoError(t, err)
	assert.Equal(t, "张三", values[0])
	assert.Equal(t, "zhangsan@example.com", values[1])
	assert.Nil(t, values[2]) // 不存在的 field 返回 nil

	// HGETALL —— 获取所有字段（注意性能）
	all, err := rdb.HGetAll(ctx, key).Result()
	require.NoError(t, err)
	assert.Len(t, all, 4) // name, email, age, city
	assert.Equal(t, "北京", all["city"])

	rdb.Del(ctx, key)
}

func TestHash_Incr(t *testing.T) {
	key := testKey("hash:cart:user1001")

	// HINCRBY —— 对 field 做原子整数加减（购物车数量增减）
	rdb.HSet(ctx, key, "sku:1001", 2, "sku:2005", 1)

	rdb.HIncrBy(ctx, key, "sku:1001", 1)  // +1 → 3
	rdb.HIncrBy(ctx, key, "sku:2005", -1) // -1 → 0

	val, _ := rdb.HGet(ctx, key, "sku:1001").Result()
	assert.Equal(t, "3", val)

	val, _ = rdb.HGet(ctx, key, "sku:2005").Result()
	assert.Equal(t, "0", val)

	// HINCRBYFLOAT —— 浮点数加减
	rdb.HSet(ctx, key, "discount", 0.8)
	rdb.HIncrByFloat(ctx, key, "discount", -0.1)
	val, _ = rdb.HGet(ctx, key, "discount").Result()
	// 注意浮点精度问题，这里用字符串比较
	assert.Contains(t, val, "0.7")

	rdb.Del(ctx, key)

	// 【坑】HINCRBY 对不存在的 field 会从 0 开始加
	// 【坑】HINCRBYFLOAT 精度有限，金融计算建议用整数（分/厘）
}

func TestHash_FieldExistence(t *testing.T) {
	key := testKey("hash:exists")

	rdb.HSet(ctx, key, "name", "李四")

	// HEXISTS —— 检查 field 是否存在
	exists, _ := rdb.HExists(ctx, key, "name").Result()
	assert.True(t, exists)

	exists, _ = rdb.HExists(ctx, key, "email").Result()
	assert.False(t, exists)

	// HSETNX —— field 不存在时才设置（类似 String 的 SETNX）
	ok, _ := rdb.HSetNX(ctx, key, "name", "王五").Result()
	assert.False(t, ok) // name 已存在，设置失败

	ok, _ = rdb.HSetNX(ctx, key, "email", "lisi@example.com").Result()
	assert.True(t, ok) // email 不存在，设置成功

	val, _ := rdb.HGet(ctx, key, "email").Result()
	assert.Equal(t, "lisi@example.com", val)

	rdb.Del(ctx, key)
}

func TestHash_DeleteField(t *testing.T) {
	key := testKey("hash:del")

	rdb.HSet(ctx, key, "a", "1", "b", "2", "c", "3")

	// HDEL —— 删除 field
	deleted, err := rdb.HDel(ctx, key, "a", "c").Result()
	require.NoError(t, err)
	assert.Equal(t, int64(2), deleted) // 删除了 2 个 field

	all, _ := rdb.HGetAll(ctx, key).Result()
	assert.Len(t, all, 1)
	assert.Equal(t, "2", all["b"])

	rdb.Del(ctx, key)
}

func TestHash_IterateFields(t *testing.T) {
	key := testKey("hash:scan")

	// 写入大量 field
	fields := make(map[string]any)
	for i := range 100 {
		fields[string(rune('a'+i%26))+string(rune('0'+i/26))] = i
	}
	rdb.HSet(ctx, key, fields)

	// HSCAN —— 渐进式遍历，不会阻塞 Redis
	var scanned []string
	cursor := uint64(0)
	for {
		keys, nextCursor, err := rdb.HScan(ctx, key, cursor, "", 10).Result()
		require.NoError(t, err)
		scanned = append(scanned, keys...)
		if nextCursor == 0 {
			break
		}
		cursor = nextCursor
	}

	// HSCAN 会返回重复 key（每次扫描边界处可能重复），需要去重
	assert.NotEmpty(t, scanned)

	// 【坑】HGETALL 在 field 很多时会阻塞 Redis（单线程！），生产环境用 HSCAN
	// 【坑】HSCAN 返回的 cursor=0 才算遍历完，不要用 len 结果判断是否结束

	rdb.Del(ctx, key)
}

func TestHash_MemoryEfficiency(t *testing.T) {
	// 对比演示：Hash vs 多个 String key
	// 10 个 String key: 10 个 dictEntry + 10 个 SDS(key) + 10 个 SDS(value)
	// 1 个 Hash key: 1 个 dictEntry + 1 个 SDS(key) + listpack(紧凑)

	hashKey := testKey("hash:memory:obj")
	stringPrefix := testKey("string:memory:")

	// 用 Hash 存储
	rdb.HSet(ctx, hashKey,
		"name", "测试用户",
		"email", "test@example.com",
		"phone", "13800138000",
		"age", "25",
		"city", "上海",
	)

	// 用 String 存储（对比）
	rdb.Set(ctx, stringPrefix+"name", "测试用户", 0)
	rdb.Set(ctx, stringPrefix+"email", "test@example.com", 0)
	rdb.Set(ctx, stringPrefix+"phone", "13800138000", 0)
	rdb.Set(ctx, stringPrefix+"age", "25", 0)
	rdb.Set(ctx, stringPrefix+"city", "上海", 0)

	// MEMORY USAGE 查看内存占用
	hashMem, _ := rdb.MemoryUsage(ctx, hashKey).Result()
	// String 方式总共 5 个 key
	var strTotalMem int64
	for _, suffix := range []string{"name", "email", "phone", "age", "city"} {
		mem, _ := rdb.MemoryUsage(ctx, stringPrefix+suffix).Result()
		strTotalMem += mem
	}

	t.Logf("Hash 内存: %d bytes", hashMem)
	t.Logf("String 总内存: %d bytes", strTotalMem)
	t.Logf("内存节省: %.1f%%", float64(strTotalMem-hashMem)/float64(strTotalMem)*100)

	// Hash 通常更省内存，因为：
	// 1. 只有一个 key 的 SDS 开销
	// 2. field 用 listpack 连续存储，无 hash table 开销
	// 3. 字段数 ≤512 且值 ≤64 字节时，listpack 极其紧凑

	rdb.Del(ctx, hashKey)
	for _, suffix := range []string{"name", "email", "phone", "age", "city"} {
		rdb.Del(ctx, stringPrefix+suffix)
	}
}

//go:build redis

package redis_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// ============================================================================
// Bitmap 位图 — 基于 String 的位操作
// ============================================================================
//
// 【为什么用 Bitmap】
// Bitmap 本质是 String 类型的位操作（SETBIT/GETBIT/BITCOUNT）。
// 用 1 bit 存储一个布尔状态，极省内存——4 字节（32 bits）可存 32 个用户的状态。
// 百万级用户的签到/活跃统计，只需要几 MB 内存。
//
// 【适用场景】
//   - 用户签到/打卡：SETBIT sign:2026-06:uid1001 dayIndex 1
//   - 在线状态统计：SETBIT online:2026-06-28 uid 1
//   - 功能开关/Feature Flag：SETBIT feature:dark_mode userId 1
//   - 布隆过滤器（底层）：多个 Bitmap 做交集/并集
//   - 用户行为画像：SETBIT user:1001:behavior 3 1（行为ID=3 为 true）
//   - 批量统计：BITCOUNT 快速统计"已签到用户数"
//
// 【坑和注意事项】
//   1. Bitmap 只能存 0/1，不能存其他值；需要存多种状态请用多个 Bitmap
//   2. 最大 512 MB（Bitmap 底层是 String）
//   3. SETBIT 对不存在的 key 会自动创建并扩展，可能产生大量零字节
//   4. BITOP 运算结果存在目标 key 中，不会修改源 key
//   5. offset 是比特位偏移（不是字节偏移），最大 2^32 - 1
//   6. BITCOUNT 的统计范围是字节，不是比特
//   7. 大 offset（如 1000000）会导致底层 String 扩展，可能产生大内存分配

func TestBitmap_BasicOps(t *testing.T) {
	key := testKey("bitmap:checkin")

	// SETBIT —— 设置指定 offset 的比特位
	// 第 5 天签到（offset=4，因为从 0 开始）
	rdb.SetBit(ctx, key, 4, 1)
	// 第 10 天签到
	rdb.SetBit(ctx, key, 9, 1)
	// 第 15 天签到
	rdb.SetBit(ctx, key, 14, 1)

	// GETBIT —— 获取指定 offset 的比特位
	val, _ := rdb.GetBit(ctx, key, 4).Result()
	assert.Equal(t, int64(1), val) // 已签到

	val, _ = rdb.GetBit(ctx, key, 5).Result()
	assert.Equal(t, int64(0), val) // 未签到

	// BITCOUNT —— 统计值为 1 的比特位数量（签到天数）
	count, _ := rdb.BitCount(ctx, key, nil).Result()
	assert.Equal(t, int64(3), count)

	rdb.Del(ctx, key)
}

func TestBitmap_DailyCheckin(t *testing.T) {
	// 场景：按天统计用户签到（一个月 31 天）
	userID := "user:1001"
	month := "2026-06"
	key := testKey("bitmap:checkin:" + userID + ":" + month)

	// 模拟签到：第 1、5、15、28 天签到
	days := []int64{0, 4, 14, 27} // offset = dayIndex (从 0 开始)
	for _, day := range days {
		rdb.SetBit(ctx, key, day, 1)
	}

	// 统计签到天数
	totalDays, _ := rdb.BitCount(ctx, key, nil).Result()
	assert.Equal(t, int64(4), totalDays)

	// 检查某天是否签到
	for day := range int64(31) {
		val, _ := rdb.GetBit(ctx, key, day).Result()
		if day == 0 || day == 4 || day == 14 || day == 27 {
			assert.Equal(t, int64(1), val, "Day %d should be checked in", day+1)
		} else {
			assert.Equal(t, int64(0), val, "Day %d should not be checked in", day+1)
		}
	}

	// 连续签到天数（从今天往前数）
	// 这需要应用层逻辑，Bitmap 本身不支持
	// 非连续签到（1,5,15,28）：从 28 往前数，27 未签到，streak = 1
	streak := 0
	for day := int64(27); day >= 0; day-- {
		val, _ := rdb.GetBit(ctx, key, day).Result()
		t.Logf("Day %d: %d\n", day, val)
		if val == 1 {
			streak++
		} else {
			break
		}
	}
	assert.Equal(t, 1, streak) // 非连续签到，连续签到天数为 1

	rdb.Del(ctx, key)

	// 【坑】Bitmap 按天统计时，每天一个 Bitmap 太浪费；按用户分 key 更合理
	// 【坑】BITCOUNT 只统计值为 1 的位数，不区分"从未设置"和"设为 0"
}

func TestBitmap_DailyCheckin_Consecutive(t *testing.T) {
	// 场景：连续签到 4 天（第 25~28 天）
	userID := "user:1002"
	month := "2026-06"
	key := testKey("bitmap:checkin:" + userID + ":" + month)

	// 连续签到：第 25、26、27、28 天
	days := []int64{24, 25, 26, 27} // offset = dayIndex (从 0 开始)
	for _, day := range days {
		rdb.SetBit(ctx, key, day, 1)
	}

	totalDays, _ := rdb.BitCount(ctx, key, nil).Result()
	assert.Equal(t, int64(4), totalDays)

	// 连续签到天数（从今天往前数）
	streak := 0
	for day := int64(27); day >= 0; day-- {
		val, _ := rdb.GetBit(ctx, key, day).Result()
		t.Logf("Day %d: %d\n", day, val)
		if val == 1 {
			streak++
		} else {
			break
		}
	}
	assert.Equal(t, 4, streak) // 连续签到 4 天（28,27,26,25）

	rdb.Del(ctx, key)
}

func TestBitmap_OnlineStatistics(t *testing.T) {
	// 场景：统计某天的活跃用户数（UV）
	date := "2026-06-28"
	key := testKey("bitmap:active:" + date)

	// 用户访问时 SETBIT
	users := []int64{1001, 1002, 1003, 1005, 1008}
	for _, uid := range users {
		rdb.SetBit(ctx, key, uid, 1)
	}

	// BITCOUNT 统计 UV
	uv, _ := rdb.BitCount(ctx, key, nil).Result()
	assert.Equal(t, int64(5), uv) // 5 个活跃用户

	// BITOP 做交集/并集：统计连续两天都活跃的用户
	date2 := "2026-06-29"
	key2 := testKey("bitmap:active:" + date2)
	retentionUsers := []int64{1001, 1003, 1008, 1010} // 1001,1003,1008 连续两天活跃
	for _, uid := range retentionUsers {
		rdb.SetBit(ctx, key2, uid, 1)
	}

	// BITOP AND：连续两天都活跃的用户
	intersectKey := testKey("bitmap:intersect:" + date + ":" + date2)
	rdb.BitOpAnd(ctx, intersectKey, key, key2)
	retentionUV, _ := rdb.BitCount(ctx, intersectKey, nil).Result()
	assert.Equal(t, int64(3), retentionUV) // 1001, 1003, 1008

	// BITOP OR：两天任意一天活跃的用户
	unionKey := testKey("bitmap:union:" + date + ":" + date2)
	rdb.BitOpOr(ctx, unionKey, key, key2)
	totalUV, _ := rdb.BitCount(ctx, unionKey, nil).Result()
	assert.Equal(t, int64(6), totalUV) // 5 + 1 = 6（1010 只在 day2）

	rdb.Del(ctx, key, key2, intersectKey, unionKey)
}

func TestBitmap_FeatureFlags(t *testing.T) {
	// 场景：功能开关——为特定用户启用新功能
	feature := "dark_mode"
	key := testKey("bitmap:feature:" + feature)

	// 为用户 1001 和 1003 启用暗黑模式
	rdb.SetBit(ctx, key, 1001, 1)
	rdb.SetBit(ctx, key, 1003, 1)

	// 检查用户是否启用了该功能
	val, _ := rdb.GetBit(ctx, key, 1001).Result()
	assert.Equal(t, int64(1), val) // 启用

	val, _ = rdb.GetBit(ctx, key, 1002).Result()
	assert.Equal(t, int64(0), val) // 未启用

	// BITCOUNT 统计启用了该功能的用户数
	enabledCount, _ := rdb.BitCount(ctx, key, nil).Result()
	assert.Equal(t, int64(2), enabledCount)

	rdb.Del(ctx, key)

	// 【坑】功能开关需要配合灰度发布逻辑，Bitmap 只做"开关"判断
	// 【坑】用户 ID 不能为负数，offset 必须 >= 0
	// 【坑】用户 ID 太大（> 2^32）无法用 Bitmap，考虑用 Bloom Filter
}

func TestBitmap_BitPos(t *testing.T) {
	key := testKey("bitmap:pos")

	// 设置一些比特位
	rdb.SetBit(ctx, key, 3, 1)  // 00001000
	rdb.SetBit(ctx, key, 7, 1)  // 10001000
	rdb.SetBit(ctx, key, 15, 1) // 10001000 00000000

	// BITPOS —— 查找第一个值为指定值的比特位
	// 返回第一个值为 1 的 bit 位置
	// BitPos(ctx, key, bit, pos...) where pos = [start, end] in bytes
	pos, _ := rdb.BitPos(ctx, key, 1, 0, -1).Result()
	assert.Equal(t, int64(3), pos) // 第 3 位是第一个 1

	// BITPOS with byte range：指定搜索范围（字节范围）
	// 只搜索第 1 字节（byte index 1 = bit 8~15），找到 bit 15
	pos, _ = rdb.BitPos(ctx, key, 1, 1, 1).Result()
	assert.Equal(t, int64(15), pos)

	// BITFIELD —— 高级位操作（类似结构体操作）
	// 可以读取/设置指定偏移和长度的位段
	val, _ := rdb.BitField(ctx, key, "GET", "u4", 0).Result()
	// 从 offset 0 开始取 4 bits: 0000 = 0
	_ = val

	rdb.Del(ctx, key)
}

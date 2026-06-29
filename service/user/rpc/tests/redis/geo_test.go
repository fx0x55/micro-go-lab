//go:build redis

package redis_test

import (
	"fmt"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const asc = "ASC"

// ============================================================================
// Geo 地理位置 — 基于 Sorted Set 的地理位置服务
// ============================================================================
//
// 【为什么用 Geo】
// Redis Geo 底层是 Sorted Set（score = geohash 编码），支持：
//   - 存储经纬度坐标
//   - 计算两点间的距离
//   - 查找附近的点（GEOSEARCH / GEORADIUS）
//   - 获取坐标的 geohash 编码
//
// 【适用场景】
//   - 附近的人/店铺：基于用户位置查找附近的餐厅、酒店
//   - 外卖/打车配送：匹配最近的骑手/司机
//   - 地理围栏：检测用户是否在某个区域内
//   - LBS 社交：附近的人、附近动态
//   - 物流追踪：查找最近的仓库/网点
//
// 【坑和注意事项】
//   1. 精度有限：geohash 用 52 位整数表示，精度约 0.6m（赤道附近）
//   2. GEOSEARCH 比 GEORADIUS 更推荐（Redis 6.2+），GEORADIUS 已弃用
//   3. GEOSEARCH 加 WITHCOORD/WITHDIST 选项可返回距离和坐标；GEOSEARCHSTORE 将结果存入新 key
//   4. 大范围搜索（如方圆 1000km）性能较差，建议限制搜索范围
//   5. Geo 底层是 Sorted Set，可以配合 ZRANGE 等命令使用
//   6. 坐标存储在 Sorted Set 的 member 中，可以通过 GEOPOS 获取
//   7. 不支持更新坐标（需要先删除再添加）

func TestGeo_BasicOps(t *testing.T) {
	key := testKey("geo:stores")

	// GEOADD —— 添加地理位置（经度, 纬度, member）
	// 北京天安门：116.397128, 39.916527
	// 北京故宫：116.397026, 39.918058
	// 北京国贸：116.460496, 39.908735
	added, err := rdb.GeoAdd(ctx, key,
		&redis.GeoLocation{Name: "天安门", Longitude: 116.397128, Latitude: 39.916527},
		&redis.GeoLocation{Name: "故宫", Longitude: 116.397026, Latitude: 39.918058},
		&redis.GeoLocation{Name: "国贸", Longitude: 116.460496, Latitude: 39.908735},
	).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(3), added)

	// GEOPOS —— 获取位置坐标
	pos, _ := rdb.GeoPos(ctx, key, "天安门", "不存在").Result()
	require.Len(t, pos, 2)
	assert.NotNil(t, pos[0])
	assert.InDelta(t, 116.397128, pos[0].Longitude, 0.001)
	assert.InDelta(t, 39.916527, pos[0].Latitude, 0.001)
	assert.Nil(t, pos[1]) // 不存在的位置

	rdb.Del(ctx, key)
}

func TestGeo_Distance(t *testing.T) {
	key := testKey("geo:distance")

	rdb.GeoAdd(ctx, key,
		&redis.GeoLocation{Name: "北京", Longitude: 116.405285, Latitude: 39.904989},
		&redis.GeoLocation{Name: "上海", Longitude: 121.472644, Latitude: 31.231706},
		&redis.GeoLocation{Name: "广州", Longitude: 113.280637, Latitude: 23.125178},
	)

	// GEODIST —— 计算两点间距离
	// 单位：m（米）、km（公里）、mi（英里）、ft（英尺）
	dist, _ := rdb.GeoDist(ctx, key, "北京", "上海", "km").Result()
	t.Logf("北京到上海: %.1f km", dist)
	assert.True(t, dist > 1000 && dist < 1200) // 大约 1068km

	dist, _ = rdb.GeoDist(ctx, key, "北京", "广州", "km").Result()
	t.Logf("北京到广州: %.1f km", dist)
	assert.True(t, dist > 1800 && dist < 2000) // 大约 1890km

	rdb.Del(ctx, key)

	// 【坑】GEODIST 精度受 geohash 影响，两点距离 < 1m 时可能不准确
}

func TestGeo_SearchByRadius(t *testing.T) {
	key := testKey("geo:radius")

	// 模拟：几家咖啡店的位置
	rdb.GeoAdd(ctx, key,
		&redis.GeoLocation{Name: "星巴克(国贸)", Longitude: 116.461, Latitude: 39.909},
		&redis.GeoLocation{Name: "瑞幸(大望路)", Longitude: 116.470, Latitude: 39.910},
		&redis.GeoLocation{Name: "Manner(CBD)", Longitude: 116.455, Latitude: 39.912},
		&redis.GeoLocation{Name: "Costa(三里屯)", Longitude: 116.453, Latitude: 39.933},
	)

	// GeoSearchLocation — go-redis 方法，内部调用 GEOSEARCH + WITHCOORD/WITHDIST（Redis 6.2+）
	results, err := rdb.GeoSearchLocation(ctx, key, &redis.GeoSearchLocationQuery{
		GeoSearchQuery: redis.GeoSearchQuery{
			Member:     "Manner(CBD)",
			Radius:     3,
			RadiusUnit: "km",
			Sort:       asc,
			Count:      10,
			CountAny:   false,
		},
		WithCoord: true,
		WithDist:  true,
		WithHash:  false,
	}).Result()
	require.NoError(t, err)
	t.Logf("搜索结果（按距离排序）:")
	for _, r := range results {
		t.Logf("  %s - 距离: %.2f km", r.Name, r.Dist)
	}

	// 带 WITHCOUNT 只返回最近 2 个
	results2, _ := rdb.GeoSearchLocation(ctx, key, &redis.GeoSearchLocationQuery{
		GeoSearchQuery: redis.GeoSearchQuery{
			Member:     "Manner(CBD)",
			Radius:     5,
			RadiusUnit: "km",
			Sort:       asc,
			Count:      2,
			CountAny:   false,
		},
		WithDist: true,
	}).Result()
	assert.LessOrEqual(t, len(results2), 2)

	rdb.Del(ctx, key)
}

func TestGeo_SearchByBox(t *testing.T) {
	key := testKey("geo:box")

	// 模拟几个快递网点
	rdb.GeoAdd(ctx, key,
		&redis.GeoLocation{Name: "网点A", Longitude: 116.400, Latitude: 39.910},
		&redis.GeoLocation{Name: "网点B", Longitude: 116.410, Latitude: 39.920},
		&redis.GeoLocation{Name: "网点C", Longitude: 116.420, Latitude: 39.900},
	)

	// GeoSearchLocation BYBOX — 在矩形范围内搜索（go-redis 封装 GEOSEARCH + WITHCOORD/WITHDIST）
	results, _ := rdb.GeoSearchLocation(ctx, key, &redis.GeoSearchLocationQuery{
		GeoSearchQuery: redis.GeoSearchQuery{
			Member:    "网点A",
			BoxWidth:  5,
			BoxHeight: 5,
			BoxUnit:   "km",
			Sort:      asc,
			Count:     10,
			CountAny:  false,
		},
		WithDist: true,
	}).Result()
	t.Logf("矩形搜索结果:")
	for _, r := range results {
		t.Logf("  %s - 距离: %.2f km", r.Name, r.Dist)
	}

	rdb.Del(ctx, key)

	// 【坑】BYBOX 的宽高是以 Member 为中心的矩形，不是圆形
	// 【坑】矩形搜索比圆形搜索更灵活，但计算开销略大
}

func TestGeo_SearchByCoordinates(t *testing.T) {
	key := testKey("geo:coords")

	rdb.GeoAdd(ctx, key,
		&redis.GeoLocation{Name: "网点A", Longitude: 116.400, Latitude: 39.910},
		&redis.GeoLocation{Name: "网点B", Longitude: 116.420, Latitude: 39.905},
	)

	// GeoSearchLocation 从坐标点搜索（不依赖已有的 member，内部调用 GEOSEARCH FROMLONLAT）
	results, _ := rdb.GeoSearchLocation(ctx, key, &redis.GeoSearchLocationQuery{
		GeoSearchQuery: redis.GeoSearchQuery{
			Longitude:  116.410,
			Latitude:   39.910,
			Radius:     5,
			RadiusUnit: "km",
			Sort:       asc,
			Count:      5,
			CountAny:   false,
		},
		WithDist: true,
	}).Result()

	assert.NotEmpty(t, results)
	for _, r := range results {
		t.Logf("  %s - 距离: %.2f km", r.Name, r.Dist)
	}

	rdb.Del(ctx, key)
}

func TestGeo_Geohash(t *testing.T) {
	key := testKey("geo:hash")

	rdb.GeoAdd(ctx, key,
		&redis.GeoLocation{Name: "天安门", Longitude: 116.397128, Latitude: 39.916527},
	)

	// GEOHASH —— 获取 geohash 编码（字符串形式，可用于前缀匹配）
	hashes, _ := rdb.GeoHash(ctx, key, "天安门").Result()
	require.Len(t, hashes, 1)
	t.Logf("天安门 geohash: %s", hashes[0])
	// geohash 长度越长精度越高
	// 5 位: ~5km 精度
	// 8 位: ~20m 精度
	// 12 位: ~0.6m 精度

	// 底层是 Sorted Set，可以用 ZRANGE 查看所有 member
	rdb.ZRangeWithScores(ctx, key, 0, -1)

	rdb.Del(ctx, key)

	// 【坑】geohash 在极地附近精度好，赤道附近精度差
	// 【坑】geohash 有"边界问题"：两个相邻但分属不同 geohash 块的点可能被判定为远
}

func TestGeo_UpdateLocation(t *testing.T) {
	key := testKey("geo:update")

	// 初始位置
	rdb.GeoAdd(ctx, key,
		&redis.GeoLocation{Name: "司机:1001", Longitude: 116.400, Latitude: 39.910},
	)

	// 更新位置需要先删后加（Geo 不支持直接更新）
	rdb.ZRem(ctx, key, "司机:1001")
	rdb.GeoAdd(ctx, key,
		&redis.GeoLocation{Name: "司机:1001", Longitude: 116.450, Latitude: 39.920},
	)

	// 验证位置已更新
	pos, _ := rdb.GeoPos(ctx, key, "司机:1001").Result()
	require.NotNil(t, pos[0])
	assert.InDelta(t, 116.450, pos[0].Longitude, 0.001)

	// 计算移动距离
	dist, _ := rdb.GeoDist(ctx, key, "司机:1001", "司机:1001", "m").Result()
	assert.InDelta(t, 0.00, dist, 0.00) // 同一点距离为 0

	rdb.Del(ctx, key)

	// 【坑】先删后加不是原子操作！并发更新可能丢失位置
	// 【坑】高频更新位置的场景（如打车），考虑用 Hash 存坐标 + Geo 做搜索
}

func TestGeo_NearbyDrivers(t *testing.T) {
	// 模拟：打车场景——查找最近的 3 个司机
	key := testKey("geo:drivers")

	// 司机位置（随机分布在北京）
	rdb.GeoAdd(ctx, key,
		&redis.GeoLocation{Name: fmt.Sprintf("driver:%d", 1001), Longitude: 116.400, Latitude: 39.910},
		&redis.GeoLocation{Name: fmt.Sprintf("driver:%d", 1002), Longitude: 116.420, Latitude: 39.905},
		&redis.GeoLocation{Name: fmt.Sprintf("driver:%d", 1003), Longitude: 116.410, Latitude: 39.915},
		&redis.GeoLocation{Name: fmt.Sprintf("driver:%d", 1004), Longitude: 116.450, Latitude: 39.930},
		&redis.GeoLocation{Name: fmt.Sprintf("driver:%d", 1005), Longitude: 116.395, Latitude: 39.908},
	)

	// 用户位置：国贸
	userLon, userLat := 116.460, 39.909

	// GeoSearchLocation 从坐标点搜索，返回最近 3 个司机
	results, err := rdb.GeoSearchLocation(ctx, key, &redis.GeoSearchLocationQuery{
		GeoSearchQuery: redis.GeoSearchQuery{
			Longitude:  userLon,
			Latitude:   userLat,
			Radius:     5,
			RadiusUnit: "km",
			Sort:       asc,
			Count:      3,
			CountAny:   false,
		},
		WithDist: true,
	}).Result()
	require.NoError(t, err)
	assert.LessOrEqual(t, len(results), 3)

	t.Logf("最近的 3 个司机:")
	for i, r := range results {
		t.Logf("  %d. %s - 距离: %.2f km", i+1, r.Name, r.Dist)
	}

	// 附近所有司机（100km 范围）
	allResults, _ := rdb.GeoSearchLocation(ctx, key, &redis.GeoSearchLocationQuery{
		GeoSearchQuery: redis.GeoSearchQuery{
			Longitude:  userLon,
			Latitude:   userLat,
			Radius:     100,
			RadiusUnit: "km",
			Sort:       asc,
			Count:      0, // 不限制数量
			CountAny:   false,
		},
		WithDist: true,
	}).Result()
	t.Logf("100km 范围内共 %d 个司机", len(allResults))

	rdb.Del(ctx, key)
}

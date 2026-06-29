# Redis 集成测试

本目录包含 Redis 各数据结构的集成测试，用于验证 Redis 功能的正确性。

## 运行测试

```bash
# 执行所有 Redis 测试（需要本地 Redis）
go test -tags=redis ./service/user/rpc/tests/redis/...

# 运行某个数据结构的测试
go test -tags=redis ./service/user/rpc/tests/redis/... -run TestGeo

# 指定 Redis 地址（默认 localhost:6379）
REDIS_ADDR=192.168.1.100:6379 go test -tags=redis ./service/user/rpc/tests/redis/...
```

## 前置条件

- Redis 服务运行中
- 默认连接 `localhost:6379`，可通过 `REDIS_ADDR` 环境变量覆盖
- 测试使用 DB 15 隔离数据，测试结束后自动清理

## 测试覆盖

| 文件 | 数据结构 | 说明 |
|------|----------|------|
| `string_test.go` | String | 字符串操作 |
| `hash_test.go` | Hash | 哈希表操作 |
| `list_test.go` | List | 列表操作 |
| `set_test.go` | Set | 集合操作 |
| `zset_test.go` | Sorted Set | 有序集合操作 |
| `bitmap_test.go` | Bitmap | 位图操作 |
| `geo_test.go` | Geo | 地理位置操作 |
| `hyperloglog_test.go` | HyperLogLog | 基数统计 |
| `pubsub_test.go` | Pub/Sub | 发布订阅 |
| `stream_test.go` | Stream | 消息流操作 |

## 为什么使用 build tag？

这些测试需要真实的 Redis 服务，不适合在单元测试中默认执行。使用 `//go:build redis` 标签可以：

- 避免 CI 环境中因缺少 Redis 而失败
- 单元测试运行更快（跳过集成测试）
- 需要时显式启用：`go test -tags=redis`
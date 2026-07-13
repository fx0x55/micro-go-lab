# Kafka 迁移计划：用 Kafka 替换 Redis 消息队列

> 本文档是一份**自包含的执行计划**，供新的对话上下文直接实施，无需重新探索代码库或重新做设计决策。所有 17 个设计决策已通过 grilling 流程确认。

## 目标

引入 **Apache Kafka**（KRaft 模式，3 节点集群）替换项目当前的 **Redis Streams** 消息队列职责。Redis 本身保留（仍用于 cache / 限流 / idempotency gate），仅移除其 MQ 角色。

学习目标：在生产者 / 消费者分离的拓扑中体验 Kafka 的分区、副本（ISR）、消费者组重平衡、offset 提交、重放与幂等消费。

---

## 已确认的设计决策（17 项）

| # | 决策 | 选定值 |
|---|------|--------|
| 1 | Go SDK | **`github.com/segmentio/kafka-go`**（纯 Go，API 成熟稳定） |
| 2 | Kafka 版本/部署 | **3 节点 KRaft 集群**，`apache/kafka:4.x`（无 ZooKeeper） |
| 3 | 交付语义 | **保留 DB outbox + Poller**，仅替换传输层；at-least-once + 消费者去重 |
| 4 | 消费者 | **构建真实跨服务消费者**（不再是空转） |
| 5 | 消费者位置 | **在 `order-api` 内**，不新增服务（保持服务数量少） |
| 6 | 记录结构 | **key=EventKey**（按 user_id 分区），**value=Payload**（JSON Envelope），**headers=元数据**（event_id/event_type/version/occurred_at） |
| 7 | 迁移策略 | **全量替换 `common/xstream/`** 为 Kafka 实现；保留 `xredis`/`xcache`/限流 |
| 8 | Topic 拓扑 | **3 partitions, RF=3, min.insync.replicas=2, 7 天 retention**；启动时显式创建（IF-MISSING） |
| 9 | Compose | **双 listener 3 节点**，同时加入 `docker-compose.yml` 和 `docker-dev.yml`；app 通过 `KAFKA_BOOTSTRAP_SERVERS` 连接 |
| 10 | 配置 | `KafkaConfig{BootstrapServers, GroupID, Topic}`；`KAFKA_BOOTSTRAP_SERVERS` env override |
| 11 | Producer | **同步 Produce**，幂等 producer，`RequireAllISRAcks`，`MaxAttempts: 5` |
| 12 | Consumer | AutoCommitMarks 等价（MarkMessage 后自动提交）+ `FirstOffset`（从最早）+ rebalance 钩子（Setup/Cleanup） |
| 13 | 消费者副作用 | **`known_users` 物化视图**（CQRS），通过现有 `IdempotentRepository` 去重 |
| 14 | Topic 声明 | `xevent` 包集中定义 `TopicSpecs()`；Producer 启动时 ensure |
| 15 | 指标 | `RedisStreamMessagesConsumed` → `KafkaMessagesConsumed`（加 `partition` label）；保留 `OutboxEventsPublished` |
| 16 | 测试 | 重写 4 个 `xstream` 集成测试为 Kafka 等价；`//go:build integration`；跳过跨服务 e2e |
| 17 | 命名 | 保留包名 `xstream`（重写内部）；保留 `tests/redis/*`；清理过期注释 |

---

## SDK 选择：segmentio/kafka-go 关键 API 映射

> 因 franz-go 使用者相对少、担心踩坑，改用 segmentio/kafka-go。下面是它与前面决策的具体映射，新 context 实施时照此编码。

### Producer（Writer）

`kafka.Writer` 是 kafka-go 的同步/异步生产者。本项目用同步模式：

```go
w := &kafka.Writer{
    Addr:         kafka.TCP(brokers...),     // []string{"kafka-1:9092", ...}
    Balancer:     &kafka.Hash{},              // 按 Message.Key 分区（保证同 user_id 顺序）
    RequiredAcks: kafka.RequireAllISRAcks,   // 等同 min.insync.replicas 保证
    MaxAttempts:  5,                          // 重试次数
    Async:        false,                      // 同步：WriteMessages 阻塞直到 ack
    BatchTimeout: 10 * time.Millisecond,
}
err := w.WriteMessages(ctx, kafka.Message{
    Topic:   topic,
    Key:     []byte(eventKey),
    Value:   payload,
    Headers: []kafka.Header{
        {Key: "event_id", Value: []byte(eventID)},
        {Key: "event_type", Value: []byte(eventType)},
        ...
    },
})
w.Close()
```

幂等 producer：kafka-go 的 Writer 在 `RequiredAcks != None` 时底层自动启用幂等语义（PID + sequence number），无需额外配置。

### Consumer Group

kafka-go 用 `kafka.ConsumerGroup` + `kafka.ConsumerGroupHandler` 接口实现消费者组：

```go
type handler struct {
    handle func(ctx context.Context, msg kafka.Message) error
}

// Setup：分区被分配时调用（等同 OnPartAssigned）
func (h handler) Setup(s kafka.ConsumerGroupSession) error { return nil }

// Cleanup：分区被撤销时调用（等同 OnPartRevoked）—— 提交已 mark 的 offset
func (h handler) Cleanup(s kafka.ConsumerGroupSession) error {
    s.Commit()   // 立即提交，防止 rebalance 丢消息
    return nil
}

// ConsumeClaim：处理消息循环
func (h handler) ConsumeClaim(s kafka.ConsumerGroupSession, c kafka.ConsumerGroupClaim) error {
    for msg := range c.Channel() {
        if err := h.handle(s.Context(), msg); err != nil {
            return err // 不 mark，at-least-once 会重投
        }
        s.MarkMessage(msg, "") // mark，由 session 自动提交
    }
    return nil
}

g, _ := kafka.NewConsumerGroup(kafka.ConsumerGroupConfig{
    Brokers:  brokers,
    Topics:   []string{topic},
    GroupID:  groupID,
    StartOffset: kafka.FirstOffset, // 全新组从最早开始（重放学习）
})
err := g.Consume(ctx, handler)
```

### Topic 创建（Admin）

```go
conn, err := kafka.Dial("tcp", broker)
// 3 partitions, RF=3
conn.CreateTopics(kafka.TopicConfig{
    Topic:             name,
    NumPartitions:     3,
    ReplicationFactor: 3,
})
// 返回的 error 在 topic 已存在时需容忍（已存在 = OK）
conn.Close()
```

retention.ms 和 min.insync.replicas 通过 topic config 设置；kafka-go 的 `kafka.Conn.CreateTopics` 的 `TopicConfig` 不直接支持 config map，需要用 `kafka.Client.AlterConfigs` 或在 `CreateTopicsRequest` 中带 ConfigEntries。实施时如遇 API 限制，退而用 docker init container 跑 `kafka-topics --alter` 设置 broker/topic 级 config，或接受 broker 默认 `min.insync.replicas=1`（开发可接受，生产再调）。

---

## 代码库关键上下文（新 context 必读）

### 现有消息队列架构

生产链路（两个服务都做）：
```
DB transaction（业务写 + outbox 行）→ Poller 定时读 outbox → Producer.Publish → Redis XAdd → Stream
```

消费链路：**当前无生产代码消费**。`xstream.Consumer` 类型存在且有单测，但没有任何服务接入。`IdempotentRepository` 同样已构建+测试但无生产调用。

### 关键文件与契约

**`common/xstream/`**（本次全量重写为 Kafka）：
- `stream.go` — `Producer`（封装 `*redis.Client`）、`Consumer`（XReadGroup/XAck/XAutoClaim）。**全部删除替换为 kafka-go 实现**。
- `poller.go` — `Poller`（定时读 outbox → `producer.Publish`）。**逻辑保留**，仅改 `Publish` 调用签名。
- `recover.go` — `RunWithRecover`。**保留不动**（panic 恢复与 broker 无关）。
- `stream_test.go` — `//go:build integration`，4 个测试。**重写为 Kafka 等价**。

当前 `Producer.Publish` 签名（要替换）：
```go
func (p *Producer) Publish(ctx context.Context, stream string, values map[string]string) (string, error)
```

当前 `Poller.tick` 构建 values map（poller.go 第 88-95 行）：
```go
values := map[string]string{
    "event":       event.EventType,
    "event_id":    event.EventID,
    "event_key":   event.EventKey,
    "version":     strconv.Itoa(event.Version),
    "occurred_at": event.CreatedAt.Format(time.RFC3339),
    "payload":     event.Payload,
}
_, err := p.producer.Publish(ctx, event.Topic, values)
```

**`common/xevent/`**（保留，少量扩展）：
- `event.go` — `TopicUserEvents="user-events"`, `TopicOrderEvents="order-events"`，`Envelope` 结构，`EventType` 常量。**新增 `TopicSpecs()` 返回 topic 拓扑定义**。
- `outbox.go` — `OutboxEvent` 结构（含 `EventID, Topic, EventKey, EventType, Version, Payload, Status, RetryCount...`）。**保留不动**。
- `outbox_repo.go` — `OutboxRepository`（Insert/MarkAsSent/MarkAsFailed/IncrementRetryCount/FindPending）。**保留不动**。
- `idempotent.go` — `IdempotentRepository.Process(eventID, fn)`。**首次被生产代码调用**（在 order-api 消费者中）。

**`common/xmetrics/xmetrics.go`**：
- `OutboxEventsPublished{topic, result}` — broker 无关，**保留**。
- `RedisStreamMessagesConsumed{topic, group, result}` — **重命名为 `KafkaMessagesConsumed`**，加 `partition` label：`{topic, group, partition, result}`。

**`common/config/types.go`**：
- 现有 `RedisConfig`、`DatabaseConfig` 等。**新增 `KafkaConfig`**：
```go
type KafkaConfig struct {
    BootstrapServers []string
    GroupID          string `json:",optional"` // 消费者才需要
    Topic            string `json:",optional"` // 消费者才需要
}
```

**服务 ServiceContext（两处改）**：
- `service/user/rpc/internal/svc/servicecontext.go` — 当前构造 `Producer`(redis) + `Poller`。**改为构造 Kafka Producer + Poller**。user-rpc 是纯生产者，不需要 Consumer。
- `service/order/api/internal/svc/serviceContext.go` — 当前构造 `Producer`(redis) + `Poller`。**改为 Kafka Producer + Poller + Consumer**（消费 `user-events`）。新增 `Start()` 方法启动 Consumer goroutine（仿 user-rpc 的 `Start()`）。

**服务 config（两处改）**：
- `service/user/rpc/internal/config/config.go` — 加 `Kafka KafkaConfig` + `ApplyEnvOverrides` 读 `KAFKA_BOOTSTRAP_SERVERS`。
- `service/order/api/internal/config/config.go` — 同上。
- `user-api` **不加 Kafka**（纯网关，不碰 MQ）。

**生产者业务逻辑（不动事件结构，注释更新）**：
- `service/user/rpc/internal/logic/createuserlogic.go` — 写 `OutboxEvent{Topic: TopicUserEvents, EventKey: user_id, ...}`。**不改**。
- `service/order/api/internal/logic/order/createOrderLogic.go` — 写 `OutboxEvent{Topic: TopicOrderEvents, EventKey: user_id, ...}`。**不改**。

**迁移**：
- `common/xdb/migrations/order/00001_init.sql` — 已含 `processed_events` 表（幂等去重就绪）。
- **新增 `common/xdb/migrations/order/00002_known_users.sql`** — `known_users` 物化视图表。

**配置文件（两处改）**：
- `service/user/rpc/etc/user-rpc.yaml` — 加 `Kafka:` 段。
- `service/order/api/etc/order-api.yaml` — 加 `Kafka:` 段（含 GroupID）。

**基础设施（两文件改）**：
- `docker-compose.yml` — 加 3 节点 KRaft kafka，service env 加 `KAFKA_BOOTSTRAP_SERVERS`。
- `docker-dev.yml` — 同样加 3 节点 KRaft（用于 `make infra`）。

**Makefile** — `make dev-*` 目标通过 env 注入 `KAFKA_BOOTSTRAP_SERVERS=localhost:9094,localhost:9095,localhost:9096`。

---

## 实现步骤清单（按依赖顺序）

### 阶段 1：基础设施与依赖

- [ ] **1.1** `go get github.com/segmentio/kafka-go@latest`
- [ ] **1.2** 在 `docker-dev.yml` 加 3 节点 KRaft Kafka 集群（双 listener：PLAINTEXT 内网 `9092`、EXTERNAL localhost `9094/9095/9096`）。每个节点 `KAFKA_PROCESS_ROLES=broker,controller`，`KAFKA_CONTROLLER_QUORUM_VOTERS` 指向全部 3 节点的 9093。
- [ ] **1.3** 在 `docker-compose.yml` 同样加 3 节点（供全栈容器化），service 的 env 加 `KAFKA_BOOTSTRAP_SERVERS: kafka-1:9092,kafka-2:9092,kafka-3:9092`。
- [ ] **1.4** 验证集群启动：`make infra` 后用 kafka-go 或 `kcat` 列 topic / 描述 cluster。

### 阶段 2：配置层

- [ ] **2.1** `common/config/types.go` 加 `KafkaConfig` 结构体。
- [ ] **2.2** `service/user/rpc/internal/config/config.go`：Config 加 `Kafka commonconfig.KafkaConfig`；`ApplyEnvOverrides` 读 `KAFKA_BOOTSTRAP_SERVERS`（逗号分隔）。
- [ ] **2.3** `service/order/api/internal/config/config.go`：同上（Config 加 `Kafka`，含 GroupID/Topic）。
- [ ] **2.4** `etc/user-rpc.yaml` 加 `Kafka:` 段（仅 BootstrapServers）。
- [ ] **2.5** `etc/order-api.yaml` 加 `Kafka:` 段（BootstrapServers + GroupID: `order-api` + Topic: `user-events`）。

### 阶段 3：xevent 扩展

- [ ] **3.1** `common/xevent/event.go` 加 `TopicSpec` 结构 + `TopicSpecs()` 函数，集中定义两个 topic 的拓扑（3 partitions, RF=3, retention 7d）。修正 `Envelope` 注释（"Redis Stream 消息" → "Kafka 消息"）。

### 阶段 4：xstream 重写（核心）

- [ ] **4.1** 重写 `common/xstream/stream.go`：
  - 定义 `Message{Topic, Key, Value, Headers}` 结构。
  - `Producer` 改为封装 `*kafka.Writer`。`NewProducer(bootstrapServers []string) (*Producer, error)` 内部 ensure topics（调 `ensureTopics`）+ 建 Writer。
  - `Publish(ctx, msg Message) error`（同步 WriteMessages）。
  - `Close()`。
  - `ensureTopics(bootstrapServers, specs)`：用 `kafka.Conn.CreateTopics` 创建（已存在则忽略），尽力设置 topic config。
- [ ] **4.2** 重写 `common/xstream/stream.go` 的 `Consumer`：
  - 封装 `kafka.ConsumerGroup`。
  - `ConsumerConfig{Brokers, Topic, Group}`。
  - `Handler func(msg Message) error`。
  - 内部 `kafka.ConsumerGroupHandler` 实现：`Setup`/`Cleanup(Commit)`/`ConsumeClaim(MarkMessage)`。
  - `Start(ctx, wg)` 启动 goroutine 跑 `g.Consume(ctx, handler)`，ctx 取消退出。
  - 分区信息从 `kafka.Message.Partition` 取，传给 metrics。
- [ ] **4.3** 改 `common/xstream/poller.go` 的 `tick`：把构建 `map[string]string` 改为构建 `Message{Topic, Key, Value, Headers}`，调 `producer.Publish(ctx, msg)`。更新注释。
- [ ] **4.4** `recover.go` 不动。

### 阶段 5：指标

- [ ] **5.1** `common/xmetrics/xmetrics.go`：`RedisStreamMessagesConsumed` → `KafkaMessagesConsumed{topic, group, partition, result}`。更新 init 注册。
- [ ] **5.2** 新 Consumer 调用点用新指标。

### 阶段 6：order-api 消费者 + known_users

- [ ] **6.1** 新增迁移 `common/xdb/migrations/order/00002_known_users.sql`：
```sql
-- +goose Up
CREATE TABLE `known_users` (
    `user_id`   BIGINT UNSIGNED NOT NULL PRIMARY KEY,
    `username`  VARCHAR(64) NOT NULL,
    `seen_at`   DATETIME(3) NOT NULL DEFAULT NOW(3),
    INDEX `idx_known_users_username` (`username`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
-- +goose Down
DROP TABLE IF EXISTS `known_users`;
```
- [ ] **6.2** `service/order/api/internal/model/` 加 `KnownUser` GORM 模型 + repository（`Upsert(tx, *KnownUser)`）。
- [ ] **6.3** `service/order/api/internal/svc/serviceContext.go`：
  - 加 `Consumer *xstream.Consumer`、`IdempotentRepo *xevent.IdempotentRepository`、`KnownUserRepo`。
  - 构造 Kafka Producer（替换 redis producer）+ Poller + Consumer（消费 `user-events`）。
  - 加 `Start()` 方法启动 Consumer goroutine（仿 user-rpc）。`order.go` main 调 `svcCtx.Start()`。
  - `Stop()` 里 close consumer。
- [ ] **6.4** 消费 handler：解析 `Envelope`（从 `Message.Value`），取 header `event_id`，调 `idempotentRepo.Process(eventID, func(tx) { knownUserRepo.Upsert(tx, ...) })`。打日志 + 自增 `KafkaMessagesConsumed{...,"success"|"skip"|"error"}`。

### 阶段 7：user-rpc 改造

- [ ] **7.1** `service/user/rpc/internal/svc/servicecontext.go`：Producer 改 Kafka 构造（`xstream.NewProducer(cfg.Kafka.BootstrapServers)`）。移除对 redis 的 MQ 依赖（redis 仅留 cache）。`Poller` 用新 Producer。

### 阶段 8：Compose env & Makefile

- [ ] **8.1** `docker-compose.yml`：user-rpc / order-api 加 `KAFKA_BOOTSTRAP_SERVERS` env。
- [ ] **8.2** `docker-dev.yml`：确保 kafka 节点在（阶段 1 已加）。
- [ ] **8.3** Makefile：`dev-user-rpc` / `dev-order-api` 目标注入 `KAFKA_BOOTSTRAP_SERVERS=localhost:9094,localhost:9095,localhost:9096`（仿 `REDIS_HOST` 模式，或在 yaml 默认 localhost）。

### 阶段 9：测试

- [ ] **9.1** 重写 `common/xstream/stream_test.go`（`//go:build integration`）：
  - `TestProducerPublish` — 发布后用 admin 验证 topic 有消息。
  - `TestConsumerStartReceiveMessages` — 发 N 条 → 消费 N 条。
  - **`TestConsumerRebalanceTakeover`**（原 ClaimPending）— 消费者 A 读了不 mark → 消费者 B 加入组 → rebalance → B 接管分区重投。**最有教学价值**。
  - `TestConsumerReplayDedup`（原 ReceiveAfterRestart）— 提交 offset → 重启 → FirstOffset 重放 → 通过 IdempotentRepository 跳过已处理。
  - TestMain 连 `localhost:9094`，用唯一 topic 前缀隔离。

### 阶段 10：注释清理

- [ ] **10.1** 全局搜 "Redis Stream" 相关注释，更新为 "Kafka topic"：`poller.go`、`createuserlogic.go`、`createOrderLogic.go`、`event.go`、README 架构图。

### 阶段 11：README 更新

- [ ] **11.1** README 架构图：Redis 框的 "Stream" 职责移到新的 Kafka 框。基础设施表加 Kafka 行。

---

## 验证方法

1. `make infra` → 3 节点 Kafka 集群健康。
2. `make dev-user-rpc` + `make dev-order-api` 启动。
3. 调 user-api 注册用户 → 查 `orders_db.known_users` 出现新行（Kafka → order-api 消费 → CQRS 物化视图）。
4. 重启 order-api → `FirstOffset` 重放 → 日志显示已处理事件被 IdempotentRepository 跳过（"skip"）。
5. 停一个 Kafka 节点 → 观察集群仍可用（RF=3 容忍 2 故障），ISR 收缩可见。
6. 跑 `go test -tags integration ./common/xstream/...` 全绿。
7. Grafana / `curl :9102/metrics | grep kafka_messages` 看到消费计数，按 partition 分。

---

## 注意事项

- **outbox 表不动**：`OutboxEvent.EventKey` 字段已是 user_id 字符串，直接作为 Kafka record key，分区保证同用户顺序。
- **幂等 producer**：kafka-go Writer 在 `RequiredAcks=AllISRAcks` 下底层启用幂等语义，不需额外配置。
- **topic config 限制**：如 kafka-go 的 `CreateTopics` API 无法直接设 `min.insync.replicas` / `retention.ms`，退而用 broker 默认或 init container `kafka-configs` 设置（开发可接受默认）。
- **保留 `tests/redis/*`**：这些是学习 Redis 数据结构的练习，不碰 `xstream`，迁移后仍可跑（redis 还在）。
- **不加跨服务 e2e 测试**：`known_users` 行本身就是端到端验证，包级集成测试已足够。

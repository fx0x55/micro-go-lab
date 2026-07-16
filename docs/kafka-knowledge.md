# Kafka 知识体系：从原理到 Go 实战（kafka-go + Kafka 4.x）

> 面向 Golang 开发者，使用 `github.com/segmentio/kafka-go`，broker 为 **Apache Kafka 4.x（KRaft 模式）**。
> 本文档是 **自包含、循序渐进** 的学习手册：从"是什么"到"怎么用"再到"怎么用对"，最后落到本项目（`micro-go-lab`）的真实实现。
>
> 阅读路线：第 1–2 章建立概念 → 第 3 章理解架构 → 第 4–5 章掌握生产/消费 → 第 6–8 章吃透语义与可靠性 → 第 9–10 章进阶事务 → 第 11–12 章是可运行的 kafka-go 实战 → 第 13 章对照本项目 → 第 14–16 章运维与排坑。

---

## 目录

1. [Kafka 是什么，解决什么问题](#1-kafka-是什么解决什么问题)
2. [核心概念与数据模型](#2-核心概念与数据模型)
3. [集群架构与 KRaft（Kafka 4.x 的关键变化）](#3-集群架构与-kraftkafka-4x-的关键变化)
4. [生产者机制](#4-生产者机制)
5. [消费者与消费者组](#5-消费者与消费者组)
6. [交付语义：at-most-once / at-least-once / exactly-once](#6-交付语义at-most-once--at-least-once--exactly-once)
7. [Offset 管理与重放](#7-offset-管理与重放)
8. [可靠性、副本与顺序保证](#8-可靠性副本与顺序保证)
9. [幂等生产者与事务](#9-幂等生产者与事务)
10. [Topic 与分区的运维考量](#10-topic-与分区的运维考量)
11. [kafka-go 实战 API](#11-kafka-go-实战-api)
12. [一个可运行的端到端示例](#12-一个可运行的端到端示例)
13. [本项目（micro-go-lab）的落地对照](#13-本项目micro-go-lab的落地对照)
14. [安全（SASL / TLS）](#14-安全sasl--tls)
15. [性能调优速查](#15-性能调优速查)
16. [常见坑与排错清单](#16-常见坑与排错清单)
17. [附录：kafka-go 速查表与 Kafka 4.x 兼容性](#17-附录kafka-go-速查表与-kafka-4x-兼容性)

---

## 1. Kafka 是什么，解决什么问题

Apache Kafka 是一个**分布式的事件流平台**（event streaming platform）。它本质上是一个**以日志（log）为数据结构的、可水平扩展的、可复制的、持久化的消息系统**。可以把它拆成三句话来理解它的定位：

- **发布/订阅消息流**——生产者写消息，多个消费者各自按自己的节奏读取。
- **持久化的日志存储**——消息不是"读完即删"，而是像数据库一样按时间顺序追加保存一段时间，可重放。
- **在消息到达时实时处理**——既是队列，也是存储，还能流式处理。

### 1.1 与常见 MQ 的关键差异

| 维度 | Kafka | RabbitMQ / RocketMQ（传统队列） | Redis Streams |
|------|-------|--------------------------------|---------------|
| 数据模型 | 分区有序日志 | 队列 / 主题 + 交换器 | 内存日志 |
| 消费模型 | 拉取（pull）+ offset | 推送（push）为主 | 拉取 + 消费组 |
| 消息持久 | 按 retention 长期保留，可重放 | 多为消费即删除 | 内存 + RDB/AOF，容量受限 |
| 吞吐 | 极高（顺序写盘 + 零拷贝） | 中高 | 高但受限于单机内存 |
| 顺序保证 | 分区内严格有序 | 队列内有序 | 流内有序 |
| 回溯/重放 | 原生支持（移动 offset） | 不支持（消息已删） | 部分支持 |

一句话记忆：**Kafka 是"用日志思维重新设计的队列 + 存储系统"，不是传统 MQ。**

### 1.2 典型适用场景

- **解耦 / 异步通信**：服务之间通过事件通信，削峰填谷（本项目的 user/order/inventory 事件流即是此类）。
- **事件溯源（Event Sourcing）与 CQRS**：事件是不可变的事实记录，消费端构建物化视图（本项目用 `known_users` 物化视图）。
- **日志/指标/审计聚合**：海量数据采集后供下游流处理或批处理。
- **数据管道（ETL）**：连接数据库 CDC 与数据仓库/湖。

**不擅长**：需要复杂路由（topic-to-queue 路由选 RabbitMQ）、单消息极低延迟的 RPC 式请求/应答、消息体超大且需按消息级别优先级。

---

## 2. 核心概念与数据模型

理解 Kafka 的前提，是建立一套**精确的词汇表**。请逐个吃透：

### 2.1 Topic（主题）

逻辑上的消息分类。类似数据库里的"表"。一个 topic 会被切分成多个 partition。本项目定义了三个 topic：`user-events`、`order-events`、`inventory-events`。

### 2.2 Partition（分区）

topic 的**物理分片**，是 Kafka 伸缩与并行度的基本单位。每个 partition 是一个**不可变的、有序的、不断追加**的日志（append-only log）。

- **顺序性**：单 partition 内消息严格按写入顺序排列。**跨 partition 不保证顺序**。
- **并行度**：一个 topic 的最大并行消费者数 = partition 数。partition 数决定上限。
- **伸缩**：不同 partition 可以分布在不同 broker 上，从而水平扩展。

### 2.3 Offset（位移）

partition 内每条消息的**单调递增整数编号**，从 0 开始。它标识"这条消息在分区日志里的位置"。消费者通过移动 offset 来控制自己读到哪了。

> Offset 是**分区内**的概念：`(topic, partition, offset)` 三元组唯一确定一条消息。

### 2.4 Record（消息）的结构

一条 Kafka 消息由四部分组成：

| 字段 | 说明 | 本项目用法 |
|------|------|-----------|
| `Key` | 可选，用于**分区路由**与日志压缩 | `user_id`、`order_id` 等业务键 |
| `Value` | 消息体（字节数组） | JSON 序列化的 `Envelope` |
| `Headers` | 键值对元数据（kafka 0.11+） | `event_id`、`event_type`、`version`、`occurred_at` |
| `Timestamp` | 消息时间戳 | 创建/追加时间 |

**Key 的核心作用：相同 key 的消息一定进同一个 partition，因此同一实体的相关事件保持有序。** 本项目用 `&kafka.Hash{}` 按 key 哈希分区，保证"同一用户的全部事件"落在同一分区。

### 2.5 Segment（日志分段）

一个 partition 在磁盘上由多个 **segment 文件**组成（`.log` 数据 + `.index` 索引 + `.timeindex` 时间索引）。当前活跃 segment 写满（或到时间）后滚动出一个新 segment。这是 Kafka 顺序写盘、区间删除、快速查找的基础。

### 2.6 消费者组（Consumer Group）

一组共同分担消费工作的消费者。**核心规则：一个 partition 在一个 group 内只能被一个消费者消费。** 这是 Kafka 实现"点对点"（group 内）与"发布订阅"（不同 group）两种模型的关键。

---

## 3. 集群架构与 KRaft（Kafka 4.x 的关键变化）

### 3.1 核心组件

- **Broker**：一个 Kafka 服务进程。多个 broker 组成集群，partition 副本分布其上。
- **Controller（控制器）**：集群中负责管理元数据、分区 leader 选举、broker 上下线的"管理者"角色。
- **ZooKeeper（历史）**：早期 Kafka 用 ZooKeeper 存储元数据、做 leader 选举。
- **KRaft（现状）**：Kafka 内置的 Raft 一致性协议，用来管理元数据，替代 ZooKeeper。

### 3.2 副本机制（Replication）

每个 partition 有一个 **leader 副本**和若干 **follower 副本**。

- **Leader**：所有读写都先打到 leader。
- **Follower**：从 leader 异步拉取数据做复制，保持与 leader 同步。
- **ISR（In-Sync Replicas）**：与 leader 保持"足够同步"的副本集合（包含 leader 自己）。只有 ISR 里的副本才有资格被选为新 leader。
- **HW（High Watermark，高水位）**：**所有 ISR 副本都已确认的位置**。消费者只能读到 HW 以下的消息。HW 保证了一致性：尚未被多数副本确认的数据对消费者不可见。
- **LEO（Log End Offset）**：每个副本日志的下一条待写位置。leader 收集所有副本的 LEO，取最小值即得到 HW。

> 记忆点：**LEO 是"写到了哪"，HW 是"对消费者安全的读到了哪"。** HW ≤ LEO。

### 3.3 Kafka 4.x：彻底告别 ZooKeeper

这是 **4.x 最重要的变化**，必须在脑海里钉死：

- **ZooKeeper 被完全移除**。KRaft 成为唯一模式，集群不再需要单独部署/维护 ZK。
- **元数据存放在内部 topic `__cluster_metadata`**，由一组 controller 通过 **Raft 协议**维护一致性。
- 节点角色：可以是 **专用 controller**（combined=false），也可以是 **broker+controller 合一**（combined=true，小型集群常用）。本项目用 3 节点合一的 KRaft 集群。
- **不再有 `zookeeper.connect`**。客户端和工具都改用 `bootstrap.servers` 连接。
- KRaft 引入了 **`metadata.version`**（特性门控），升级时需要关注。

其他 4.x 值得注意的演进（了解即可）：

- **KIP-848 新消费者组协议**：新一代 group coordinator，rebalance 更稳更快；kafka-go 当前使用的是经典消费者协议，仍被 4.x 完整支持。
- **KIP-405 分层存储（Tiered Storage）**：冷数据可下沉到对象存储，broker 本地只留热数据。
- **KIP-932 共享组（Share Groups）**：面向队列语义的"竞争消费"模式（早期访问）；kafka-go 尚未支持，本项目也不需要。

> 对开发者的结论：**在 4.x 里你只需关心 broker 地址、topic/分区、副本数；不再需要理解 ZK。** 客户端代码（kafka-go）几乎无感知。

### 3.4 一条消息从生产到被读，经历了什么

```
Producer → [路由到 partition（按 key hash）] → Leader Broker（写本地 log）
        → Followers 拉取复制 → ISR 全确认 → 推进 HW
Consumer（group）→ fetch（只能读 HW 以下）→ 处理 → 提交 offset
```

这张"数据流"贯穿后面所有章节。

---

## 4. 生产者机制

### 4.1 生产者写一条消息的内部流程

1. **序列化**：把 key/value 转成字节数组（kafka-go 里通常你自己 `json.Marshal`）。
2. **分区（Partitioning）**：由 `Balancer` 决定进哪个 partition。
3. **累积（Accumulate）**：消息先进一个"批次缓冲区"，攒一批或到时间再发，提高吞吐。
4. **发送（Send）**：把批次发给对应 partition 的 leader。
5. **确认（Ack）**：leader（及副本）按 `RequiredAcks` 决定何时回 ack。
6. **重试（Retry）**：失败按 `MaxAttempts` 重试。

### 4.2 分区器（Balancer）—— 顺序的第一道闸门

| 策略（kafka-go） | 行为 | 何时用 |
|------------------|------|--------|
| `&kafka.Hash{}` | 对 key 做哈希 → 固定分区 | **需要按 key 保序**（本项目默认） |
| `&kafka.RoundRobin{}` | 轮询各分区 | key 为空、追求均匀分布、不要求顺序 |
| `&kafka.LeastBytes{}` | 选累计字节最少的分区 | 负载均衡取向 |

> **黄金法则**：一旦你需要"同一实体的消息按发生顺序到达"，就必须给它们相同的 key 并用 `&kafka.Hash{}`。本项目把 `user_id`/`order_id` 作为 key 正是这个原因。

### 4.3 `acks`（`RequiredAcks`）—— 可靠性 vs 延迟

这是生产者最重要的可靠性参数：

| 值（kafka-go） | 含义 | 丢失风险 | 延迟 |
|----------------|------|---------|------|
| `RequireNone (0)` | 发出去就算，不等确认 | 最高（broker 宕机会丢） | 最低 |
| `RequireOne (1)` | 只等 leader 写入确认 | 中（leader 写后、复制前宕机会丢） | 中 |
| `RequireAll (-1)` | 等 ISR 全部副本确认 | **最低** | 最高 |

> **本项目用 `kafka.RequireAll`**（对应 broker 端 `acks=-1`）。要真正安全，还必须配合 `min.insync.replicas`（见第 8 章）。
>
> 注意：kafka-go v0.4.x 里**没有** `RequireAllISRAcks` 这个常量，等价的就是 `RequireAll`（值为 `-1`）。

### 4.4 批次与压缩

- **`BatchSize` / `BatchBytes` / `BatchTimeout`**：控制攒批的容量与最长等待时间。批越大吞吐越高、延迟越大。
- **`Compression`**：`gzip` / `snappy` / `lz4` / `zstd`。zstd 是综合最优的现代选择。压缩在批级别生效，能显著降低网络与存储开销。

### 4.5 重试与顺序

- **`MaxAttempts`**：重试次数。本项目设为 5。
- **重试的危险**：重试可能导致**同 key 的消息乱序**（前一条重试成功、后一条先成功）。经典消费者依赖顺序时，必须保证"同一分区内的发送顺序"。

> 如果既想重试又想严格保序，需要启用**幂等生产者**（见第 9 章）。kafka-go 的高层 `Writer` 没有直接暴露幂等开关，因此本项目改为在**消费端去重**来兜底（DB outbox + 幂等表），这是工程上很稳妥的折中。

---

## 5. 消费者与消费者组

### 5.1 消费者组的核心规则

1. 同一个 `group.id` 内，**一个 partition 只被一个消费者**消费。
2. group 内所有消费者"瓜分"该 topic 的全部 partition。
3. 不同 group 之间互相独立，各读各的（发布/订阅）。

推论：

- **消费者数 > partition 数**时，多出来的消费者空闲。
- **partition 数 = 最大并行消费能力**。想加并发，先加分区（注意分区只能加不能减）。

### 5.2 消费者组协调者与 Rebalance

- **Group Coordinator**：broker 上负责管理某个 group 的角色。
- **Rebalance（重平衡）**：group 成员变化（加入/退出/崩溃）或分区数变化时，重新把 partition 分配给消费者。

Rebalance 的代价：期间该 group **短暂停止消费**。频繁 rebalance 是常见稳定性问题，需通过 `SessionTimeout` / `HeartbeatInterval` / `RebalanceTimeout` 合理调参，并实现 `Setup`/`Cleanup` 钩子做优雅处理。

### 5.3 分区分配策略（GroupBalancers）

kafka-go 的 `ReaderConfig.GroupBalancers` 决定 partition 怎么分：

- `kafka.RangeBalancer`：按区间连续切分（默认风格之一）。
- `kafka.RoundRobinBalancer`：轮询分配，分布更均匀。

### 5.4 Offset 提交（核心中的核心）

消费者读到哪里，记录在 `__consumer_offsets` 内部 topic 里。两种提交方式：

| 方式 | 说明 | 本项目选择 |
|------|------|-----------|
| **自动提交** | 后台定时把"读到的最新 offset"提交 | 易丢/易重，**不推荐**对可靠性敏感场景 |
| **手动提交** | 处理完业务逻辑后再提交 offset | **本项目采用** |

本项目用 `FetchMessage` → 业务处理成功 → `CommitMessages` 的"处理后提交"模式，`CommitInterval: 0` 表示关闭自动提交、完全手动控制。

### 5.5 `StartOffset`（无提交位点时从哪开始）

- `kafka.FirstOffset`（= `earliest`）：从分区最早可用的消息开始读。**本项目用它**，避免新 group 错过历史事件。
- `kafka.LastOffset`（= `latest`）：只读接入后的新消息。

> 一旦 group 提交过 offset，`StartOffset` 就不再生效——Kafka 会从已提交的位点继续。`StartOffset` 只决定"全新 group 的第一次起点"。

---

## 6. 交付语义：at-most-once / at-least-once / exactly-once

这是面试与生产中最常考、也最容易出事的部分。记住：**语义由"提交 offset 的时机"决定。**

### 6.1 三种语义

| 语义 | 含义 | 实现方式（手动提交语境） | 风险 |
|------|------|------------------------|------|
| **at-most-once（最多一次）** | 消息可能丢失，但绝不重复 | **先提交 offset，再处理业务** | 处理失败则永久丢失 |
| **at-least-once（至少一次）** | 消息绝不丢失，但可能重复 | **先处理业务，成功后再提交 offset** | 处理成功但提交前崩溃 → 重复投递 |
| **exactly-once（精确一次）** | 既不丢也不重 | 幂等生产者 + 事务，或消费端幂等 | 实现复杂，性能有代价 |

### 6.2 为什么"先处理后提交"是 at-least-once

设想崩溃窗口：

```
处理业务（成功）  →  ① 这一刻崩溃  →  提交 offset
```

崩溃发生在 ①：offset 没提交 → 重启后从上次位点重读 → **这条消息会被处理两次**。所以 at-least-once **必然带来重复**。要"看起来精确一次"，必须让**处理逻辑本身幂等**。

> **结论：在工程上，最实用的组合是 `at-least-once 投递 + 消费端幂等`。这正是本项目的设计。** 用 `event_id` 作为幂等键，通过 `IdempotentRepository` 去重，等价于"业务层面的 exactly-once"。

### 6.3 自动提交为什么危险

自动提交（`enable.auto.commit=true` / `CommitInterval > 0`）按时间周期性提交"已拉取"的 offset，**和业务是否处理成功无关**：

- 拉取后、处理前崩溃 → offset 已提交 → **消息丢失**（降级成 at-most-once）。
- 这就是为什么本项目显式设 `CommitInterval: 0` 并手动提交。

---

## 7. Offset 管理与重放

### 7.1 offset 存在哪

`__consumer_offsets` 是一个内部 compacted topic，记录每个 `(group, topic, partition)` 的已提交 offset。

### 7.2 重放（Replay）—— Kafka 的杀手锏

因为消息按 retention 保留在日志里，你可以**把某个 group 的 offset 往回拨**，重新消费历史数据。常见做法：

- **换个 group**：用一个全新的 group.id 从 `earliest` 开始消费（最简单，本项目换 group 即可重放）。
- **seek 到指定位置**：用底层 `Conn.Seek` 或工具（`kafka-consumer-groups --reset-offsets`）把某 group 的位移重置。

> 注意：重放前确认 retention 还在；超过 retention 的数据已被删除，无法重放。

### 7.3 日志压缩（Log Compaction）

对于"以 key 为最新值"的 topic（如状态快照），可开启 `cleanup.policy=compact`：Kafka 会为每个 key 只保留**最新**的 value，旧的同 key 记录被清理。适合配置、状态、物化视图这类场景，而不是事件流。

---

## 8. 可靠性、副本与顺序保证

### 8.1 三剑客：`acks` + `min.insync.replicas` + `replication.factor`

要达到"生产不丢 + 高可用"，三者缺一不可：

1. **`acks=all`**（生产者侧 `RequiredAcks: kafka.RequireAll`）：等所有 ISR 确认。
2. **`min.insync.replicas`**（topic/broker 侧，本项目 = 2）：规定 ISR 至少有几个副本才算"安全可写"。
3. **`replication.factor`**（topic 侧，本项目 = 3）：总副本数。

**配合逻辑**：RF=3、min.insync.replicas=2 意味着"允许 1 个副本宕机，仍能安全写入"。如果 ISR 少于 2，生产者会收到 `NOT_ENOUGH_REPLICAS` 异常——宁可拒绝写入，也不冒丢数据的风险。

### 8.2 `unclean.leader.election`（非同步副本当选）

- `false`（推荐，4.x 默认）：只允许 ISR 副本当 leader，**牺牲可用性换数据一致**。
- `true`：允许落后的副本当选，可能丢数据。**强一致性场景务必保持 false。**

### 8.3 顺序保证的边界

| 范围 | 是否有序 | 条件 |
|------|---------|------|
| 单 partition 内 | ✅ 严格有序 | 天然保证 |
| 同 key 跨多条消息 | ✅ 有序 | key 相同 + `Hash` 分区 + 重试不破坏顺序 |
| 跨 partition / 跨 key | ❌ 无序 | 设计使然 |
| 多线程处理单分区 | ❌ 可能乱序 | 需要自己在应用层按 key 串行 |

> **多线程消费陷阱**：kafka-go 的 `Reader` 对单 partition 是串行投递的；如果你在 handler 里另起 goroutine 并行处理，就会破坏顺序与 at-least-once 的去重边界。本项目 handler **同步处理**，处理完才提交，正是为了守住这条线。

### 8.4 一张图：丢/重/乱的成因与防线

```
生产侧丢失：acks=0/1 + leader宕机未复制      → 防线: acks=all + min.insync.replicas
生产侧乱序：失败重试把后发的先成功            → 防线: 幂等生产者 / 消费端去重
消费侧丢失：自动提交后、处理前崩溃            → 防线: 手动提交(CommitInterval=0)
消费侧重投：处理成功、提交前崩溃              → 防线: 消费端幂等(event_id 去重)
跨分区乱序：本身不保证                       → 防线: 相同实体用同一 key
```

---

## 9. 幂等生产者与事务

### 9.1 幂等生产者（Single-Partition 幂等）

启用后，Kafka 给每个 producer 一个 **PID（Producer ID）**，每条消息带 **序列号（sequence number）**。broker 端去重：

- 网络抖动导致**重试**同一批 → broker 识别重复序列号 → **自动去重**。
- 解决的是"生产者重试造成的重复"，**只对单分区单会话有效**。

> kafka-go 高层 `Writer`（v0.4.51）**没有直接暴露幂等开关**。需要时可用底层 `protocol` 自行构造 `ProduceRequest`，或继续采用"消费端幂等"。本项目走后者，工程上更简单可控。

### 9.2 事务（跨分区/多 topic 的原子写入）

事务解决"**生产 A 和生产 B 要么都成功、要么都不成功**"（典型：消费 → 处理 → 产出，整套原子完成）。

核心 API（kafka-go 在 `protocol`/底层包支持，高层封装有限）：

1. `InitProducerID` —— 拿到事务 PID。
2. `BeginTransaction`
3. 正常 produce（带事务标记）
4. `AddOffsetsToTxn` + `TxnOffsetCommit` —— 把消费 offset 也纳入事务
5. `CommitTransaction`（成功）或 `AbortTransaction`（放弃）

**代价**：吞吐下降、延迟上升、实现复杂。**不要默认上事务**——大多数业务用"at-least-once + 幂等消费"已足够。

### 9.3 决策树：到底要不要 exactly-once

```
需要"消费→处理→产出"端到端精确一次吗?
├─ 否 → at-least-once + 消费端幂等  ✅(本项目, 推荐)
└─ 是 → 钱袋子/对账等强一致场景 → 考虑 producer 幂等 + 事务
```

---

## 10. Topic 与分区的运维考量

### 10.1 分区数怎么定

经验法则：**单分区吞吐估算后，分区数 = 期望总吞吐 ÷ 单分区吞吐**，并预留余量。同时考虑：

- **= 最大消费者并发数**（消费者不会超过分区数）。
- **= key 的并行度上限**（同 key 必须同分区，key 很多才能拆细）。
- 过多分区 → broker 元数据/请求开销上升、端到端延迟略增。
- 本项目 3 partition，与 3 节点集群匹配，预留扩展空间。

### 10.2 分区只能加不能减

**一旦扩容，key → partition 的映射会变化**（hash(key) mod N 改变），导致同一 key 的历史消息在新旧分区之间"断裂"。所以：

- 依赖 key 顺序的场景，**尽早把分区数定够**。
- 如果必须扩容且在意顺序，考虑用新 topic 双写过渡。

### 10.3 Retention（保留策略）

- `retention.ms`：按时间保留，超期删除整个 segment（本项目 7 天）。
- `retention.bytes`：按大小保留。
- 删除单位是 **segment**，不是单条消息，所以不会精确到秒删除。

### 10.4 Topic 声明：IF-MISSING

本项目在 producer 启动时**显式创建 topic**（已存在则忽略错误），避免依赖 broker 的自动创建（生产环境通常关闭 `auto.create.topics.enable`）。见 `ensureTopics`。

---

## 11. kafka-go 实战 API

> 库：`github.com/segmentio/kafka-go` v0.4.51。它提供三层 API：高层 `Writer`/`Reader`、底层 `Conn`、最底层 `protocol`。日常用高层即可。

### 11.1 生产者：`kafka.Writer`

```go
package main

import (
	"context"
	"time"

	"github.com/segmentio/kafka-go"
)

func newProducer(brokers []string) *kafka.Writer {
	return &kafka.Writer{
		Addr:         kafka.TCP(brokers...), // 多个 broker 地址，kafka-go 会发现全集群
		Topic:        "user-events",         // 可在 Writer 级固定，也可在 Message 级覆盖
		Balancer:     &kafka.Hash{},         // 按 Message.Key 分区，保同 key 有序
		RequiredAcks: kafka.RequireAll,      // acks=-1，等全部 ISR 确认
		MaxAttempts:  5,                     // 失败重试次数
		Async:        false,                 // 同步：WriteMessages 阻塞到收到 ack
		BatchTimeout: 10 * time.Millisecond, // 攒批最长等待
		// Compression: kafka.Snappy,        // 需要时开启压缩
	}
}

// 写一条消息
func publish(ctx context.Context, w *kafka.Writer, key, value []byte) error {
	return w.WriteMessages(ctx, kafka.Message{
		Key:   key, // 路由与保序的关键
		Value: value,
		Headers: []kafka.Header{
			{Key: "event_id", Value: []byte("evt-123")},
			{Key: "event_type", Value: []byte("user.registered")},
		},
	})
}
```

关键点：

- `kafka.TCP(brokers...)` 接受多个地址，连上任意一个后即可发现全集群，但给全量地址更稳。
- `Async: false` 时 `WriteMessages` 返回错误即代表写入失败/未确认。
- `Async: true` 时错误通过 `Completion` 回调上报，**必须处理，否则失败被静默吞掉**。
- **不要每条消息 new 一个 Writer**：Writer 会维护连接与缓冲，应作为长生命周期单例，关闭时 `Close()`。

### 11.2 消费者：`kafka.Reader`（消费组 + 手动提交）

```go
func consume(ctx context.Context, brokers []string) error {
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:          brokers,
		Topic:            "user-events",
		GroupID:          "order-user-events", // 关键：设了 GroupID 才是消费组模式
		StartOffset:      kafka.FirstOffset,   // 全新 group 从最早开始（已提交位点后失效）
		CommitInterval:   0,                   // 0 = 关闭自动提交，改为手动 CommitMessages
		MinBytes:         1,                   // 至少 1 字节就返回，降低延迟
		MaxBytes:         10e6,                // 单次最多 ~10MB
		SessionTimeout:   10 * time.Second,
		RebalanceTimeout: 10 * time.Second,
		HeartbeatInterval: 3 * time.Second,
	})
	defer r.Close() // defer 必须有，否则 group 不会及时释放分区

	for {
		// FetchMessage 不会自动提交 offset，需要我们处理成功后手动提交
		m, err := r.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil // 优雅退出
			}
			continue
		}

		if err := handle(m); err != nil {
			// 处理失败：不提交 offset → at-least-once 重投
			continue
		}
		// 处理成功：手动提交，落位 __consumer_offsets
		if err := r.CommitMessages(ctx, m); err != nil {
			// 提交失败通常不致命，下次启动会重读，幂等逻辑兜底
		}
	}
}
```

两种读取方法的区别：

| 方法 | 是否自动提交 | 适用 |
|------|-------------|------|
| `r.ReadMessage(ctx)` | **是**（按 `CommitInterval` 自动提交） | 简单场景，但语义弱 |
| `r.FetchMessage(ctx)` + `r.CommitMessages(ctx, m...)` | **否**，完全手动 | **本项目选择**，可控的 at-least-once |

### 11.3 底层 `kafka.Conn`（管理/特殊操作）

用于创建 topic、查元数据、按 partition seek 等：

```go
import (
	"net"
	"strconv"

	"github.com/segmentio/kafka-go"
)

// 连到任意 broker，再定位到 controller
conn, _ := kafka.Dial("tcp", "kafka-1:9092")
defer conn.Close()
ctrl, _ := conn.Controller()
ctrlAddr := net.JoinHostPort(ctrl.Host, strconv.Itoa(ctrl.Port))

// 连 controller 创建 topic（已存在则忽略）
topicConn, _ := kafka.Dial("tcp", ctrlAddr)
defer topicConn.Close()
_ = topicConn.CreateTopics(kafka.TopicConfig{
	Topic:             "user-events",
	NumPartitions:     3,
	ReplicationFactor: 3,
})
```

### 11.4 分区分配与 rebalance 钩子

`ReaderConfig.GroupBalancers` 控制分配策略。需要"优雅 rebalance"（比如 Cleanup 时 flush 缓冲）时，kafka-go 的高层 `Reader` 能力有限，复杂场景可退到自行管理分区或使用 `protocol` 层。

---

## 12. 一个可运行的端到端示例

下面是一段**自包含**的最小示例，演示"生产一条 → 消费组消费 → 手动提交"的完整闭环。`go.mod` 引入 `github.com/segmentio/kafka-go v0.4.51`，broker 用本地 4.x KRaft（如 `localhost:9092`）。

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/segmentio/kafka-go"
)

func main() {
	brokers := []string{"localhost:9092"}
	topic := "demo-events"
	group := "demo-group"

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 1) 生产者
	w := &kafka.Writer{
		Addr:         kafka.TCP(brokers...),
		Topic:        topic,
		Balancer:     &kafka.Hash{},
		RequiredAcks: kafka.RequireAll,
		MaxAttempts:  5,
		BatchTimeout: 10 * time.Millisecond,
	}
	defer w.Close()

	// 2) 消费者（FetchMessage + 手动提交）
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:           brokers,
		Topic:             topic,
		GroupID:           group,
		StartOffset:       kafka.FirstOffset,
		CommitInterval:    0, // 手动提交
		SessionTimeout:    10 * time.Second,
		RebalanceTimeout:  10 * time.Second,
		HeartbeatInterval: 3 * time.Second,
		MinBytes:          1,
		MaxBytes:          10e6,
	})
	defer r.Close()

	// 3) 生产几条（同一 key 保序）
	go func() {
		for i := 0; i < 5; i++ {
			err := w.WriteMessages(ctx, kafka.Message{
				Key:   []byte("user-42"), // 同 key → 同 partition → 有序
				Value: []byte(fmt.Sprintf("hello-%d", i)),
				Headers: []kafka.Header{
					{Key: "event_id", Value: []byte(fmt.Sprintf("evt-%d", i))},
				},
			})
			if err != nil && ctx.Err() == nil {
				log.Printf("produce err: %v", err)
			}
			time.Sleep(200 * time.Millisecond)
		}
	}()

	// 4) 消费循环
	for {
		m, err := r.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("fetch err: %v", err)
			time.Sleep(time.Second)
			continue
		}
		fmt.Printf("p%d@%d key=%s value=%s\n", m.Partition, m.Offset, m.Key, m.Value)
		if err := r.CommitMessages(ctx, m); err != nil {
			log.Printf("commit err: %v", err)
		}
	}
}
```

跑起来后你会看到：相同 key 的消息落在同一个 partition、offset 连续递增，进程重启后从已提交位点继续。

---

## 13. 本项目（micro-go-lab）的落地对照

本项目用"**DB Outbox + Poller 推送到 Kafka + 消费组幂等消费**"的真实拓扑，是上面理论的完整落地。对照如下：

### 13.1 数据结构约定（见 `common/xevent`）

- **Topic**：`user-events` / `order-events` / `inventory-events`，均 3 分区 RF=3，retention 7 天。
- **消息格式**：`key=EventKey`（如 `user_id`）、`value=JSON Envelope{event, version, occurred_at, data}`、`headers=event_id/event_type/version/occurred_at`。

### 13.2 生产者（见 `common/xstream/stream.go`）

```go
w := &kafka.Writer{
    Addr:         kafka.TCP(bootstrapServers...),
    Balancer:     &kafka.Hash{},      // 按业务 key 分区 → 同实体有序
    RequiredAcks: kafka.RequireAll,   // acks=-1
    MaxAttempts:  5,
    Async:        false,              // 同步写入
    BatchTimeout: 10 * time.Millisecond,
}
```

`Publish` 同步写入一条 `xstream.Message{Topic, Key, Value, Headers}`；`ensureTopics` 在启动时按 `TopicSpecs()` 显式建 topic（已存在忽略）。

### 13.3 消费者（见 `common/xstream/stream.go` 的 `consume`）

关键设计，逐条对应前文理论：

- `GroupID` 设定 → 消费组模式，group 内分区互斥分配。
- `StartOffset: kafka.FirstOffset` → 新 group 不漏历史事件。
- `CommitInterval: 0` + `FetchMessage` + `CommitMessages` → **处理成功后才提交**，at-least-once 语义。
- handler 失败时**不提交** → 触发重投。
- 带指数退避的重试循环（`backoff`，上限 `maxBackoff`）→ 应对 fetch 失败。
- 指标 `KafkaMessagesConsumed{topic,group,partition,status}` → 观察 success/error/skip。

### 13.4 幂等消费（见各 `*_event_consumer.go`）

消费端用 `event_id`（来自 header）作为幂等键，经 `IdempotentRepository.Process` 在事务内去重，再做副作用（如 `UpsertKnownUser`）。这正是把 at-least-once 升级为"业务精确一次"的关键。参见 [stream.go](/Users/pundix002/wokoworks/go/micro-go-lab/common/xstream/stream.go) 与 [user_event_consumer.go](/Users/pundix002/wokoworks/go/micro-go-lab/service/order/api/internal/svc/user_event_consumer.go)。

### 13.5 可靠性配置总结

| 关注点 | 本项目做法 | 对应章节 |
|--------|-----------|---------|
| 生产不丢 | outbox 落库 + 同步 produce + `RequireAll` | 4.3 / 13.2 |
| broker 高可用 | 3 节点 KRaft，RF=3，min.insync.replicas=2 | 3.2 / 8.1 |
| 消费不丢 | 手动提交（`CommitInterval:0`），失败不提交 | 6.3 / 13.3 |
| 消费不重 | `event_id` 幂等去重 | 6.2 / 13.4 |
| 保序 | key 路由 + Hash 分区 + 同步处理 | 2.4 / 4.2 / 8.3 |

---

## 14. 安全（SASL / TLS）

生产环境通常要求鉴权与加密。kafka-go 通过 `Dialer` 配置：

```go
import (
	"crypto/tls"

	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl/scram"
)

mech, _ := scram.Mechanism(scram.SHA512, "user", "password")
dialer := &kafka.Dialer{
	SASLMechanism: mech,
	TLS:           &tls.Config{ /* ServerName, RootCAs ... */ },
}

r := kafka.NewReader(kafka.ReaderConfig{
	Brokers: brokers,
	Topic:   topic,
	GroupID: group,
	Dialer:  dialer,
})

// Writer 同样支持：
// w := &kafka.Writer{ Addr: kafka.TCP(brokers...), /* ... */, Transport: &kafka.Transport{ TLS: &tls.Config{...}, SASL: mech } }
```

注意：Kafka 4.x 默认**未开启** SASL/TLS；是否开启取决于部署配置。本地开发（本项目）直连明文端口即可。

---

## 15. 性能调优速查

### 生产者

- 增大 `BatchSize`/`BatchTimeout` → 攒大批 → 吞吐↑、延迟↑。
- 开启 `Compression: kafka.Zstd` → 网络/存储↓。
- `RequiredAcks: RequireAll` 最稳但最慢；只在能接受风险时降级。

### 消费者

- 合理 `FetchMessage` 批量处理（攒一批再一起 `CommitMessages`）→ 减少 commit 次数。
- `MinBytes`/`MaxBytes` 调节一次拉取量；`MinBytes=1` 低延迟、`MinBytes` 较大时吞吐高。
- 消费者并发不要超过分区数。
- handler 同步处理保序；确需并行，按 key 分片到不同 worker。

### broker / topic

- 分区数覆盖吞吐与并发上限。
- `replication.factor=3` + `min.insync.replicas=2` 是可靠性与可用性的甜点。
- 关注磁盘 IO（Kafka 是顺序写，SSD 优先）与网络带宽。

---

## 16. 常见坑与排错清单

1. **消息"丢了"**：先查 `acks` 是否 `all`、`min.insync.replicas` 是否满足、是否用了自动提交（提交后崩溃丢消息）。
2. **消息重复**：at-least-once 下属正常 → 必须消费端幂等；检查是否"处理成功但提交前崩溃"。
3. **顺序乱了**：检查是否给了相同 key、是否用了非 Hash 分区、是否在 handler 内并行处理。
4. **消费者"不工作"**：消费者数 > 分区数会空闲；`GroupID` 必须设置才是消费组模式。
5. **`StartOffset` 不生效**：因为 group 已有提交位点；想从头读请换 group 或重置 offset。
6. **频繁 rebalance**：`SessionTimeout`/`HeartbeatInterval` 调参；handler 处理时间过长会超过 session 超时被踢。
7. **`defer r.Close()` 漏了**：消费组分区不会及时释放，导致重启后短暂消费停顿。
8. **每条消息 new Writer**：性能灾难；Writer 必须复用。
9. **topic 不存在**：检查 broker 是否关了自动创建（生产应关），代码里要 `ensureTopics`。
10. **分区扩容后 key 断序**：扩容改变 `hash(key) mod N`，历史与未来 key 可能落到不同分区。
11. **4.x 连不上**：确认没有再用 `zookeeper.connect`，KRaft 下只用 `bootstrap.servers`。
12. **`RequiredAcks` 用错常量**：kafka-go v0.4.x 没有 `RequireAllISRAcks`，正确值是 `kafka.RequireAll`（= -1）。

---

## 17. 附录：kafka-go 速查表与 Kafka 4.x 兼容性

### 17.1 高层 API 对照

| 概念 | kafka-go 写法 |
|------|--------------|
| 连接集群 | `kafka.TCP(brokers...)` |
| 同步生产 | `Writer{Async:false}.WriteMessages(ctx, msgs...)` |
| 分区策略 | `Balancer: &kafka.Hash{}` / `RoundRobin{}` / `LeastBytes{}` |
| acks | `RequiredAcks: kafka.RequireNone/RequireOne/RequireAll` |
| 压缩 | `Compression: kafka.Gzip/Snappy/Lz4/Zstd` |
| 消费组消费 | `Reader{GroupID:...}.ReadMessage(ctx)`（自动提交）|
| 手动提交消费 | `Reader{CommitInterval:0}` + `FetchMessage` + `CommitMessages` |
| 起始位点 | `StartOffset: kafka.FirstOffset/LastOffset` |
| 创建 topic | `Conn.CreateTopics(kafka.TopicConfig{...})` |

### 17.2 Kafka 4.x 与 kafka-go 兼容性要点

- **kafka-go 协议级客户端**，与 4.x broker 的核心 produce/consume/admin 完全兼容。
- **无需 ZooKeeper**：KRaft 下客户端只认 `bootstrap.servers`。
- **经典消费者组协议仍受 4.x 支持**：kafka-go 的 `Reader` 消费组模式正常工作；KIP-848 新协议 kafka-go 尚未采用。
- **Share Groups（KIP-932）/新特性**：kafka-go 暂不支持，本项目也不依赖。
- 常量注意：`RequireAll = -1` 即"等全部 ISR 确认"，**没有** `RequireAllISRAcks`。

---

> **延伸阅读**：本项目迁移背景与 17 项设计决策详见 [kafka-migration-plan.md](/Users/pundix002/wokoworks/go/micro-go-lab/docs/kafka-migration-plan.md)；分布式事务与 outbox/saga 思路见 [saga-distributed-transaction.md](/Users/pundix002/wokoworks/go/micro-go-lab/docs/saga-distributed-transaction.md)。
>
> 如果你的那份 ChatGPT 笔记里有本文未覆盖的点（例如 kafka-go 某个特定用法、KRaft 运维细节、某个踩坑），把片段发我，我直接补进对应章节。

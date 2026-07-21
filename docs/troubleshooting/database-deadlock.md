# 数据库死锁排查（Troubleshooting Lab）

> 这是"数据库死锁"的实操 lab。我们在 `inventory-rpc` 的预占路径里同时给出**脆弱写法**
> （`BUG_DB_DEADLOCK=1`，反序加锁 + sleep 撑大窗口）和**正确写法**（默认：统一加锁顺序 + 应用层重试），
> 用真实 MySQL 触发一次 InnoDB 死锁（1213），再走完整的"发现 → 调试 → 根因 → 防御"闭环。

## 这个 lab 的故障长什么样

库存预占涉及两张表：`products`（按 `sku` 扣减 `available`）和 `inventory_reservations`（插一条预占记录）。
预占（`Reserve`）和释放（`Release`）天然会同时碰这两张表，于是**加锁顺序**就成了死锁的命门：

- `Reserve`：先扣 `products`，再写 `inventory_reservations`
- `Release`：先改 `inventory_reservations`，再回补 `products`

两者顺序**相反**，就是一个经典的 AB-BA 隐患：两个事务交叉持锁时，InnoDB 检测到环，回滚其中一方，抛 `ERROR 1213 (40001): Deadlock found`。

- 默认（`BUG_DB_DEADLOCK=0`）：`Reserve`/`Release` 都统一成 `reservation → product` 顺序，并加 `SELECT ... FOR UPDATE` 固定锁序，外加应用层 1213 重试。见 [inventory.go](../../service/inventory/rpc/internal/repository/inventory.go) 的 `reserveOrdered`。
- 脆弱模式（`BUG_DB_DEADLOCK=1`）：`Reserve` 回退到旧的 `product → reservation` 反序，并在两步之间用 `SELECT SLEEP(?)` 撑大竞争窗口，便于复现。见 `reserveReversed`。

---

## 发现：怎么知道出事了

主信号是 [rules.yml](../../deploy/prometheus/rules.yml) `database_alerts` 组的 `DeadlockRetrySpike`：

```yaml
- alert: DeadlockRetrySpike
  expr: increase(db_deadlocks_total[5m]) > 0
  for: 2m
```

`db_deadlocks_total` 由 [retry.go](../../common/xdb/retry.go) 的 `WithRetry` 在捕获到 1213/1205 时自增，label `outcome` 区分：

- `retried` —— 死锁被捕获、退避后重试（自愈中）
- `exhausted` —— 重试次数耗尽仍失败（请求直接报错，需要止血）

```bash
curl -s --data-urlencode 'query=increase(db_deadlocks_total[5m])' \
  "http://localhost:9104/metrics" | grep db_deadlocks
# 或在 Prometheus：increase(db_deadlocks_total{outcome="exhausted"}[5m]) > 0
```

**💡 D2 锚点 — 为什么少量 `retried` 不用慌，`exhausted` 才慌：** 死锁在并发系统里是"正常事件"，InnoDB 自己检测、自己回滚、应用层重试一次通常就成功了。`retried` 在低位是健康自愈的体现；只有当它**激增**（说明加锁冲突已超出偶发），或 `exhausted` 开始涨（重试救不回来了），才意味着真出事了。

---

## 调试：揪出是哪两个事务、加锁顺序怎么反的

### 1) 抓死锁图（最权威）

本仓库已在 [docker-dev.yml](../../docker-dev.yml) / [docker-compose.yml](../../docker-compose.yml) 给 MySQL 加了
`--innodb-print-all-deadlocks=ON`：每次死锁都会把**两个**事务的完整死锁图持久化进 error log
（默认只留最后一次、还会被覆盖，排查死锁时几乎没用）。

```bash
docker compose exec mysql mysql -uroot -proot inventory_db -e "SHOW ENGINE INNODB STATUS\G" \
  | grep -A40 'LATEST DETECTED DEADLOCK'
```

输出里看三件事：
- `*** (1) TRANSACTION` / `*** (2) TRANSACTION` —— 各自最后执行的 SQL
- `*** (1) HOLDS THE LOCK(S)` / `*** (2) HOLDS THE LOCK(S)` —— 各自已持有什么锁
- `*** (1) WAITING FOR THIS LOCK` / `*** (2) WAITING FOR THIS LOCK` —— 各自在等什么锁

把"HOLDS"和"WAITING FOR"两两对上，环就出来了——那就是根因。

### 2) 在日志里定位

死锁错误会经过 [gormlogger.go](../../common/xdb/gormlogger.go) 的 `Trace`，被打了 `deadlock=true` 字段。
在 Loki 里一键过滤：

```logql
{service="inventory-rpc"} | json | deadlock="true"
```

每条都带 `trace`/`span`，拿去 Jaeger 对账就能看到是哪个订单、哪条事件触发的事务。

### 3) MySQL 实时锁（事中抓，事后就没）

死锁现场瞬间消失，但如果是**锁等待**（1205）或想看长事务持锁，可以：

```sql
-- 当前所有活跃事务
SELECT * FROM information_schema.INNODB_TRX;
-- 具体行锁
SELECT * FROM performance_schema.data_locks;
-- 谁在等谁
SELECT * FROM performance_schema.data_lock_waits;
```

---

## Step 0：复现一次

确定性复现用 `make repro-deadlock`（封装到"造事"为止，抓死锁图留给读者练）。它跑
[deadlock-repro.sh](../../deploy/mysql/deadlock-repro.sh)：开两个并发 MySQL 会话，
对 `SKU-001`/`SKU-002` 做**反序加锁**（A：先 001 后 002；B：先 002 后 001），中间用 `SLEEP` 撑大窗口，必爆 1213。

```bash
make repro-deadlock
```

预期输出里会有一条：

```
ERROR 1213 (40001): Deadlock found when trying to get lock; try restarting transaction
```

随即脚本提示去抓死锁图。

**💡 D2 锚点 — 为什么复现器用两个 SKU 的反序加锁，而不是单行：** 单行更新只会**锁等待**（一个等另一个），构不成环；死锁必须有"两把以上的锁 + 反序"。`products` 的 `sku` 是主键、`UPDATE ... WHERE sku=?` 是等值唯一索引、只加记录锁——所以单 SKU 不会自死锁。真实生产里的环往往来自**多行更新顺序不一致**或 **RR 下二级索引的 gap lock**，复现器用最干净的两行反序把它讲清楚。

### 想让应用层也吃到 1213？

以脆弱模式起 inventory-rpc，再并发下两个相同 SKU 的单（触发 `Reserve` 竞争）：

```bash
BUG_DB_DEADLOCK=1 BUG_DEADLOCK_SLEEP_MS=300 KAFKA_BOOTSTRAP_SERVERS=localhost:9094,localhost:9095,localhost:9096 \
  go run ./service/inventory/rpc
```

此时 `db_deadlocks_total` 会随重试上涨，Loki 里会出现 `deadlock="true"`。注意：因为本系统的 Kafka
事件按 `user_id` 分区，**同一用户**的 `OrderCreated`/`OrderCancelled` 会被串行化、不并发，所以应用层
死锁主要出现在跨用户、同 SKU 的高并发预占里——这本身也是一个"事件键设计如何避免死锁"的观察点。

---

## 防御 / 解决

两道防线，缺一不可。

### 第一道：统一加锁顺序（治本，治 AB-BA）

`Reserve` 和 `Release` 都改成 `reservation → product`：先 `SELECT ... FOR UPDATE` 锁住（或定序）预占位，
再动 `products`。锁序一致，环就不可能形成。见 [inventory.go](../../service/inventory/rpc/internal/repository/inventory.go)
的 `reserveOrdered` 与 `Release`。

> `Release` 的 `SELECT ... FOR UPDATE` 同时也是正确性修复：锁住即将修改的行，避免释放并发下的 lost update。

### 第二道：应用层 1213 重试（生产必备）

**即便锁序统一了，死锁也无法在应用层彻底消灭**——RR 隔离级别下，非唯一二级索引的 range 谓词会加
gap lock，两个事务的 gap 锁仍可能交叉成环（这是 MySQL 的固有行为，不是你的 bug）。所以事务外必须包一层重试：

```go
err := xdb.WithRetry(ctx, "inventory_db", func() error {
    // fn 每次都从头开一个全新事务（IdempotentRepo.Process 内部自带 db.Transaction）
    return sc.IdempotentRepo.Process(eventID, func(tx *gorm.DB) error { ... })
})
```

关键点（见 [retry.go](../../common/xdb/retry.go)）：

- **重试单元 = 整个事务**，不是事务里某条语句。死锁把事务整体回滚后，那个 `tx` 已经废了，必须在 fn 里重开。
  所以 `fn` 不接收 `*gorm.DB` 参数——从类型上就杜绝"在已回滚的 tx 上重试"。
- 只重试 1213（死锁）/ 1205（锁等待），其它错误（如 1062 唯一键冲突）立即返回，不盲目重试。
- 指数退避 + 抖动，默认 3 次、50ms 起、封顶 500ms。
- 幂等表 `processed_events` 随事务一起回滚，所以重试时去重状态是干净的——这正是"1213 重试在事件驱动架构里安全"的原因。

调用点见 [order_event_consumer.go](../../service/inventory/rpc/internal/svc/order_event_consumer.go) 的 `handleOrderEvent`。

### 其它思路（文档备选，不在本 lab 实装）

- **短事务 / 去依赖**：把 `reservations` 折叠成 `products` 上的两列（`available` + `reserved`），`Reserve` 退化成单条原子 `UPDATE`，天然无环。根治，但破坏现有一对多模型。
- **隔离级别**：降到 `READ COMMITTED` 消掉大部分 gap lock。但全局改隔离级别副作用大，只在没有更好办法时考虑，且仍需保留重试。

---

## 踩坑

- **`SHOW ENGINE INNODB STATUS` 的死锁段只有最后一次：** 默认 `LATEST DETECTED DEADLOCK` 只保留最近一次，被新的覆盖就没了。本仓库开了 `innodb_print_all_deadlocks=ON`，每次都进 error log，排查时不会扑空。
- **把重试写进事务回调里：** 常见错误是 `db.Transaction(func(tx){ WithRetry(func(){ 用 tx }) })`。tx 已经被 1213 回滚，重试再用同一个 tx 直接报错。正确写法是把 `WithRetry` 包在 `db.Transaction` **外层**。
- **盲目重试所有错误：** 只重试 1213/1205。把 1062（唯一键冲突）也重试，会把"业务上不该重试"的错误吃成无限循环。
- **`BUG_DB_DEADLOCK=1` 时 sleep 是教学道具：** 真实生产没有 sleep，但反序加锁在高并发下照样会爆——别因为"我代码里没有 sleep"就以为不会有死锁。

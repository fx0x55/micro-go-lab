# 编排式 Saga：订单 ↔ 库存分布式事务设计

> 本文档记录 order ↔ inventory 分布式事务的设计决策，是实现的依据。
> 决策通过 grilling 流程逐层锁定，共 10 个关键决策点。

## 背景

现有系统已有 user-rpc（用户域）和 order-api（订单域），下单流程中唯一的跨服务动作是
同步 gRPC `ValidateUser`——这是只读校验，没有可变副作用需要协调。事务性 Outbox
保证的是"本地事务 + 消息最终送达"（可靠消息投递），不是分布式事务。

要引入真正的分布式事务，必须新增**第二个有持久化副作用的参与方**。

## 决策清单

| # | 决策 | 选择 | 理由 |
|---|------|------|------|
| Q1 | 参与方 | **库存服务** | 订单已存在，库存是最自然的 saga 载体，补偿语义清晰（扣减/释放） |
| Q2 | 事务模式 | **编排式 Saga（事件驱动）** | 零新基础设施，复用 Kafka + Outbox + poller |
| Q3 | 库存模型 | **预留记录表** | saga_id 内建为关联键，补偿幂等天然成立 |
| Q4 | 防超卖 | **冗余 available 列 + affected rows** | 复用订单已有的乐观锁套路，风格统一 |
| Q5 | saga 范围 | **精简闭环（预占 + 释放）** | 不引入支付，happy path + compensation 一眼能看完 |
| Q6 | 订单状态机 | **不新增中间态** | 竞态靠两端 no-op + 消费幂等闭合 |
| Q7 | 缺货语义 | **自动取消订单** | 失败走事件链，与编排哲学一致 |
| Q8 | 事件契约 | **两个事件类型** | discriminated union 风格，符合 xevent 现有设计 |
| Q9 | 服务边界 | **独立服务 + 独立库，gRPC 只读** | 写全走事件，避免双写路径竞态 |
| Q10 | 基建拼装 | **继续手工** | 和 user-rpc / order-api 一致，不过早抽象 |

## 新增参与方

`service/inventory/rpc`：独立 gRPC 服务，etcd 注册，独立 `inventory_db`，own 自己的库存数据。

gRPC 只承载只读查询（展示库存可用量），所有写操作（预占/释放）只走事件，不走 gRPC。

## Saga 主流程

下单 (order-api): 本地事务写 order(pending) + outbox[OrderCreated, saga_id=order_id]，发往 Kafka order-events。

inventory 消费 OrderCreated: 库存足够则插 reservation(reserved) + 原子扣减 available + outbox[InventoryReserved]，发往 Kafka inventory-events；缺货则 outbox[ReservationFailed, reason=OUT_OF_STOCK]。

order 消费 InventoryReserved: no-op（订单仍 pending，幂等去重）。
order 消费 ReservationFailed: order(pending->cancelled, cancel_reason=OUT_OF_STOCK) + outbox[OrderCancelled]。

inventory 消费 OrderCancelled: 翻 reservation->released + 回补 available。

主动取消 (order-api): order(pending->cancelled) + outbox[OrderCancelled]，inventory 消费后释放。

## 数据模型

### inventory_db

products: 商品总量与可用量。
- sku VARCHAR(64) PK, 商品库存单位标识
- total INT NOT NULL, 总库存
- available INT NOT NULL, 可用库存（冗余列 = total - SUM(active reservations)）

inventory_reservations: 预占记录，saga_id 的一等公民。
- id BIGINT PK AUTO_INCREMENT
- order_id BIGINT NOT NULL, saga 关联键（本 saga 中 order_id 即 saga_id）
- sku VARCHAR(64) NOT NULL
- quantity INT NOT NULL, 预占数量
- status VARCHAR(32) NOT NULL DEFAULT 'reserved', 取值 reserved / released
- created_at DATETIME(3)
- UNIQUE INDEX (order_id), 一个订单只有一条预占记录

### orders_db 扩展

orders 表新增列: sku VARCHAR(64) NOT NULL, quantity INT NOT NULL DEFAULT 1,
cancel_reason VARCHAR(32) NULL（OUT_OF_STOCK / USER_CANCELLED，仅 cancelled 态有值）。

## 事件定义

Topic 拓扑（双向通信，各服务 own 自己的 topic）：

| Topic | 生产者 | 消费者 | 事件 |
|-------|--------|--------|------|
| order-events | order-api | inventory-rpc | order.created, order.cancelled |
| inventory-events | inventory-rpc | order-api | inventory.reserved, inventory.reservation_failed |

事件 payload 变更：

- OrderCreatedData 新增 Sku、Quantity 字段。
- 新增 OrderCancelledData（order_id, sku, quantity, reason）。
- 新增 InventoryReservedData（order_id）。
- 新增 ReservationFailedData（order_id, reason）。

## 关键技术约束

1. 补偿顺序：释放预占时先翻 reservation status->released，再回补 available，顺序不可反。
2. 消费端幂等：inventory 复用 IdempotentRepository.Process(eventID, fn)，order 消费
   inventory 事件同样复用，与现有 user-events 消费一致。
3. 取消事件必须带原因：order.cancelled 携带 reason（USER_CANCELLED / USER_CANCELLED），
   inventory 靠事件类型决定释放动作。
4. 竞态 no-op：order 收到 InventoryReserved 时若已是 cancelled（用户抢先取消）-> no-op；
   inventory 收到 OrderCancelled 时若无 active reservation（未预占或已释放）-> no-op。
5. inventory 自带完整 outbox 链路：Producer + Poller + Consumer + IdempotentRepo，
   与 user-rpc / order-api 的 ServiceContext 手工拼装模式一致。
6. 防超卖：预占 = 插 reservation + UPDATE products SET available = available - qty
   WHERE sku=? AND available >= qty，靠 affected rows==1 判断成败，在同一事务内。

## 扩展点（文档记录，当前不实现）

- 支付确认：支付成功后 reservation 可从 reserved 转为 committed（正式扣减），
  需引入支付域和 OrderPaid 事件。
- 对账 job：定时校验 available == total - SUM(active reservations)，发现漂移告警。

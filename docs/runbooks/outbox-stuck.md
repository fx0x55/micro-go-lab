# OutboxStuck

- 级别：warning
- 触发条件：Outbox 事件连续 10 分钟发布失败

## 现象

事务性 Outbox 的 poller 持续把事件标为发布失败，事件堆积在表中。

## 影响

下游消费者收不到事件（用户注册、订单创建等业务事件丢失或延迟）。
数据一致性风险：事务已提交但事件未发出。

## 排查步骤

1. Redis 是否可达（poller 依赖 Redis Stream）
   ```bash
   docker compose exec redis redis-cli ping
   ```
2. poller goroutine 是否存活：看日志是否有 `outbox find pending failed`
3. 看 `outbox_events` 表中 `status=failed` 的事件数与 `retry_count`

## 缓解 / 恢复

- Redis 故障：恢复 Redis，poller 下个 tick 会自动重试 pending 事件
- 超过 `MaxRetries`(5) 的事件被标记为 failed：人工排查后重置 `status=pending`

## 事后

- 是否需要 dead-letter 表而非直接 failed
- 消费侧幂等（`xevent.NewIdempotentRepository`）是否覆盖了重发场景

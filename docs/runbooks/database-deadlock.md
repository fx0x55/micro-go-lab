# DeadlockRetrySpike

- 级别：warning
- 触发条件：`increase(db_deadlocks_total[5m]) > 0` 持续 2 分钟

## 现象

`inventory-rpc`（或其它接入 `xdb.WithRetry` 的服务）的事务因 InnoDB 死锁（1213）或锁等待（1205）被回滚，正在退避重试。少量重试是正常自愈；持续激增说明加锁冲突已超出偶发范围。

## 影响

事务被回滚后退避重试，延迟随重试上升；当 `outcome="exhausted"`（重试次数耗尽）时，请求直接失败，Kafka 消息会重投。order-events 消费链的 `OrderCreated`/`OrderCancelled` 处理会受影响。

## 排查步骤

1. 拆 outcome，判断严重程度：
   ```bash
   # Prometheus：exhausted 上涨 = 重试已救不回来，需立刻止血
   increase(db_deadlocks_total{outcome="exhausted"}[5m])
   increase(db_deadlocks_total{outcome="retried"}[5m])
   ```
2. 抓死锁图（最权威），看是哪两个事务、锁顺序怎么反的：
   ```bash
   docker compose exec mysql mysql -uroot -proot inventory_db -e "SHOW ENGINE INNODB STATUS\G" \
     | grep -A40 'LATEST DETECTED DEADLOCK'
   ```
   本仓库已开 `innodb_print_all_deadlocks`，每次死锁都进了 error log，也可以在 Loki 搜：
   ```logql
   {service="mysql"} |~ "DEADLOCK|TRANSACTION"
   ```
3. 在应用日志里定位触发事务：
   ```logql
   {service="inventory-rpc"} | json | deadlock="true"
   ```
   拿 `trace` 去 Jaeger 看是哪个订单/事件。

## 缓解 / 恢复

- 重试本身在兜底：只要 `exhausted` 不持续上涨，业务多数能自愈，优先定位根因而非手动干预。
- 紧急止血：若是某条热点 SKU 引发的集中死锁，临时降低该 SKU 的并发（限流）即可让重试成功率回升。
- 根因修复后，`retried` 应回落到接近 0。

## 事后

- 是否引入了新的多写事务路径，加锁顺序是否与既有路径一致（`reservation → product`）。
- 是否有长事务持锁过久（查 `information_schema.INNODB_TRX` 里 `trx_started` 过老的）。
- 该路径是否真的需要 RR 隔离级别；必要时局部降级到 READ COMMITTED 消掉 gap lock。
- 完整流程与防御写法见 [troubleshooting/database-deadlock.md](../troubleshooting/database-deadlock.md)。

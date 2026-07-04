# CircuitBreakerOpen

- 级别：critical
- 触发条件：`increase(rpc_calls_breaker_open_total[5m]) > 0` 持续 5 分钟

## 现象

order-api 调用 user-rpc 时，go-zero 熔断器处于开启状态，请求被直接拒绝。

## 影响

所有依赖 `ValidateUser` 的下单链路降级：订单创建直接失败，用户侧表现为下单报错。

## 排查步骤

1. 确认 user-rpc 是否存活（看 ServiceDown 告警是否已触发）
2. user-rpc 慢还是错？
   - Jaeger 搜 `service=user-rpc`，看最近 span 的耗时与错误
   - Grafana 看 `user-rpc` 的 RED 指标
3. 网络/服务发现：etcd 是否正常、`user-svc.rpc` key 是否还在
   ```bash
   docker compose exec etcd etcdctl get user-svc.rpc --prefix
   ```

## 缓解 / 恢复

- user-rpc 宕机：恢复它，熔断器半开探测成功后自动闭合
- user-rpc 慢（DB 慢查询）：见 db-connection-saturation.md
- 临时：可考虑降低 order-api 对 user-rpc 的依赖（返回降级结果），但需业务确认

## 事后

- 熔断阈值是否合理（go-zero 默认 google sre 算法）
- 是否需要 fallback 策略而不是直接报错

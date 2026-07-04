# 告警处置手册（Runbooks）

每条告警规则都带 `runbook_url` 注解，指向本目录下的处置手册。
接收到告警后，值班人员按手册逐步排查；手册应随系统演进持续更新。

## 告警优先级

| 级别 | 含义 | 响应要求 |
|------|------|----------|
| critical | 服务不可用 / 数据丢失风险 | 立即响应，目标恢复时间 < 15min |
| warning | 降级 / 容量逼近 | 值班时间内处理 |
| info | 异常趋势 / 需关注 | 仅记录，可后续排期 |

## 手册索引

| 告警 | 级别 | 手册 |
|------|------|------|
| ServiceDown | critical | [service-down.md](service-down.md) |
| HighErrorRate | warning | [high-error-rate.md](high-error-rate.md) |
| HighLatencyP99 | warning | [high-latency-p99.md](high-latency-p99.md) |
| CircuitBreakerOpen | critical | [circuit-breaker-open.md](circuit-breaker-open.md) |
| DBConnectionSaturation | warning | [db-connection-saturation.md](db-connection-saturation.md) |
| OutboxStuck | warning | [outbox-stuck.md](outbox-stuck.md) |
| PanicRecoveredSpike | critical | [panic-recovered-spike.md](panic-recovered-spike.md) |

## 写手册的要求

每个手册至少包含：现象、影响、排查步骤、缓解/恢复、事后复盘项。
如果一条告警没有人知道怎么处理，它就不该存在——先补手册再开告警。

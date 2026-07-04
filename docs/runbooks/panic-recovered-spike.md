# PanicRecoveredSpike

- 级别：critical
- 触发条件：5 分钟内 `goroutine panic recovered` 日志超过 3 次（Loki ruler）

## 现象

某个后台 goroutine（poller/consumer）反复 panic。
`common/xstream.RunWithRecover` 吞掉了 panic 没让进程崩溃，但循环已失效。

## 影响

进程看起来活着（不触发 ServiceDown），但后台任务已停摆——
可能是 Outbox 不再发、Stream 不再消费，是"隐性宕机"。

## 排查步骤

1. Loki 拉日志，看 `caller` 字段（poller / consumer / ratelimiter）和 `stack`
   ```
   {service=~".+"} | json |= "goroutine panic recovered"
   ```
2. 按 stack 定位到具体代码行

## 缓解 / 恢复

- 多数情况需要重启服务让 goroutine 重新拉起
- 根因修复前，监控对应的业务指标（如 OutboxStuck）确认是否已停摆

## 事后

- panic 根因修复
- 考虑该 goroutine 是否需要自愈（panic 后重建而非仅 recover）

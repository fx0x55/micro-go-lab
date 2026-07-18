# 线上排查实操（Troubleshooting Lab）

> 教你怎么排查 Go 服务在线上的三类资源问题：CPU 热点、内存泄漏、goroutine 泄漏。
> 前两个用真实注入的 bug + 真实流量 + 真实 pprof 数据走完整流程；goroutine 那块讲套路。

## 文档

| 文档 | 形态 | 故障注入开关 |
|------|------|--------------|
| [cpu-hotspot.md](cpu-hotspot.md) | lab（有真实 profile） | `BUG_CPU=1` |
| [memory-leak.md](memory-leak.md) | lab（有真实 profile） | `BUG_MEMLEAK=1` |
| [goroutine-deadlock-quickref.md](goroutine-deadlock-quickref.md) | 速查（讲套路，不搭 lab） | — |

## 事故分流（第一秒）

```text
进程还活着？
├─ 是 → 看资源指标
│       ├─ CPU 高 → cpu-hotspot.md        → go tool pprof .../profile
│       ├─ 内存涨 → memory-leak.md         → .../debug/pprof/heap
│       └─ goroutine 涨 → goroutine-...md  → .../debug/pprof/goroutine?debug=2
└─ 否（崩了）→ 看崩溃日志最后一行
        ├─ "all goroutines are asleep - deadlock!" → goroutine-deadlock-quickref.md（二）
        └─ "signal: killed" / OOM → 系统层（dmesg / kubectl describe）
```

## 怎么跑 lab

三个 lab 都在 `order-api` 上，默认全关。需要的环境变量见各文档开头，简单说：

```bash
# CPU 热点（需要绕过 breaker + 限流）
BUG_CPU=1 BUG_LOAD_RATE=100000 go run ./service/order/api -f service/order/api/etc/order-api.yaml

# 内存泄漏
BUG_MEMLEAK=1 BUG_LOAD_RATE=100000 go run ./service/order/api -f service/order/api/etc/order-api.yaml
```

pprof 在 go-zero 的 DevServer 上，四个服务的端口：

| 服务 | pprof 端口 |
|------|-----------|
| order-api | 6060 |
| user-api | 6061 |
| user-rpc | 6062 |
| inventory-rpc | 6063 |

## profile 归档约定

所有 lab 的 profile 存 `/tmp/lab-profiles/`，按 `before / after` 命名，方便做 diff：

| 文件 | 含义 |
|------|------|
| `cpu-before.prof` | CPU 故障抓的（验证热点） |
| `cpu-after.prof` | 修复后抓的（和 before diff 确认热点消失） |
| `mem-baseline.prof` | 内存故障前的空载 baseline |
| `mem-leaking.prof` | 泄漏发生时抓的（在堆高位） |
| `mem-after.prof` | 修复后抓的 |
| `goroutine-before.txt` | goroutine profile（`debug=2` 文本格式） |

**💡 为什么强调 baseline / before：** 排查的核心方法论是"对比"，不是"看绝对值"。
单看一个 profile 分不清"正常该有的"和"问题的"。抓一份对照，diff 只看增长部分——
这是三个 slice 贯穿始终的思路（V2 验证法）。

## 告警规则

对应的 Prometheus 告警在 [rules.yml](../../deploy/prometheus/rules.yml) 的 `runtime_alerts` 组：
`HighCPUUsage` / `HeapUsageHigh` / `HeapLeakSuspected` / `GoroutineCountHigh` / `GoroutineLeakSuspected`。
每条告警的 `description` 里都带了第一步该抓哪个 profile、怎么读。

## 没覆盖的

- **goroutine 泄漏的 lab**：没注入。排查手法套路化，速查文档够用；想动手再补。
- **A 类运行时死锁的 lab**：没注入。进程会直接崩，抓不到 profile，演示价值低。
  速查文档讲了怎么读 `fatal error: all goroutines are asleep` 的堆栈。
- **非 Go 运行时的问题**（GC 调优、调度延迟、cgroup 内存限制）只作为锚点提到，
  没单独成文。

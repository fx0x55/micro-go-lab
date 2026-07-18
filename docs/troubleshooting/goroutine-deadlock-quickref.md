# Goroutine 泄漏与死锁速查（Quick Reference）

> 这块没有搭 lab（CPU/内存已经覆盖了排查方法论的核心）。直接讲手法——
> 因为 goroutine 问题的排查手段高度套路化，理解了套路，遇到真实事故照样能上手。

 先把两个概念分清，这是排查时最容易混的：

| | Goroutine 泄漏 | 运行时死锁（A 类）|
|--|----------------|------------------|
| 本质 | **部分** goroutine 卡住不退出，但进程还活着 | **所有** goroutine 全部阻塞，runtime 判定无救 |
| 进程状态 | 活着，还能处理请求（只是 goroutine 数越来越多） | **直接崩溃**，`fatal error: all goroutines are asleep - deadlock!` |
| 发现途径 | 监控告警（`go_goroutines` 涨） | 进程没了，看崩溃日志 |
| 抓 profile | 能抓（进程活着） | 抓不到（进程已死），靠日志里的完整堆栈 |

 Goroutine 泄漏是慢病，运行时死锁是猝死。排查路径完全不同。

---

## 一、Goroutine 泄漏

### 现象与告警

[rules.yml](../../deploy/prometheus/rules.yml) 的 `runtime_alerts` 组里两条，互补：

```yaml
- alert: GoroutineCountHigh         # 绝对水位
  expr: go_goroutines > 1000
  for: 5m
- alert: GoroutineLeakSuspected      # 相对增速（早期信号）
  expr: (go_goroutines - (go_goroutines offset 10m)) > 300 and on(instance) (go_goroutines > 200)
  for: 3m
```

 看趋势比看绝对值重要。`go_goroutines` 两种涨法对应两种泄漏形态：
- **单调上涨** = 慢泄漏（每请求漏一点，长期累积）
- **阶梯跳变后不回落** = 某次事件 fork 了一批 goroutine 没回收（一次性泄漏）

### Step 1：抓 goroutine profile

```bash
curl -o /tmp/lab-profiles/goroutine-before.txt \
  "http://localhost:6060/debug/pprof/goroutine?debug=2"
```

**💡 D2 锚点 — `debug=2` vs `debug=1`：**
- `debug=1`：单行汇总，每个栈只出现一次，前面带计数。适合快速扫"哪类栈最多"。
- `debug=2`：**每个 goroutine 单独列**，带完整栈 + goroutine 状态 + 阻塞原因。
  排查泄漏用这个——你能看到"卡在哪一行"。

 `debug=2` 输出长这样：

```
goroutine 12345 [chan receive, 5 minutes]:
internal/poll.runtime_pollWait(...)
net.(*pollDesc).wait(...)
net.(*conn).Read(...)
main.waitForResult(...)
    /app/foo.go:42 +0x88      ← 卡在这一行
created by main.handleRequest in goroutine 12340
    /app/foo.go:20 +0x120      ← 谁 fork 的它
```

 关键信息都在：`[chan receive]`（阻塞原因）、`5 minutes`（卡了多久）、`foo.go:42`（卡在哪）、`created by ... foo.go:20`（谁创建的）。一个泄漏 goroutine 通常长一个样，**数同一条栈出现多少次**就知道泄漏规模。

### Step 2：grep 数栈出现次数

```bash
# 提取每个 goroutine 的卡点（第一行栈）
grep -A1 "^\[goroutine [0-9]" /tmp/lab-profiles/goroutine-before.txt | \
  grep -v "^--$" | sort | uniq -c | sort -rn | head
```

 出现次数最多的那行栈，就是泄漏源。

### Step 3：diff 两次 profile（最准）

 隔 1~2 分钟抓两次：

```bash
curl -o goroutine-1.txt "http://localhost:6060/debug/pprof/goroutine?debug=2"
sleep 90
curl -o goroutine-2.txt "http://localhost:6060/debug/pprof/goroutine?debug=2"
```

 第二次比第一次**新增**的那些栈，就是泄漏的（正常 goroutine 会在间隔内退出）。
 和内存泄漏的 `-base` 思路一样：只看增长部分。

### 卡点关键字速判

 `debug=2` 里方括号 `[...]` 的状态直接告诉你卡在哪类操作：

| 状态 | 含义 | 常见根因 |
|------|------|----------|
| `[chan receive]` | 卡在 `<-ch` 接收 | 发送方跑了/没人发，或 channel 被 GC 但接收方还阻塞 |
| `[chan send]` | 卡在 `ch <- x` 发送 | 无缓冲 channel 接收方没了，或缓冲满且无人收 |
| `[select]` | 卡在 select（无 case 就绪） | select 漏了 `case <-ctx.Done()` |
| `[semacquire]` / `[sync.Mutex.Lock]` | 卡在锁 | 锁竞争或持锁者没释放 |
| `[IO wait]` / `[netpoll]` | 卡在网络读写 | HTTP client 无超时，对端不响应 |
| `[finalizer wait]` | 卡在 finalizer | 少见，finalizer 队列堆积 |

**💡 D2 锚点 — 最经典的三种泄漏：**
1. `go func() { ... <-ch }()` 漏传 context——函数返回了 goroutine 还在，ch 永远没人发 → `[chan receive]`
2. `http.Get(url)` 没设超时，对端 hang 住 → `[IO wait]`，goroutine 卡几小时
3. `for { go handle() }` 循环里 fork，无退出条件 → goroutine 数阶梯上涨

### 修复

 所有 fork 的 goroutine 都必须有**明确退出路径**，套路是统一的：
- 入口接 `ctx context.Context`，select 里带 `case <-ctx.Done(): return`
- channel 操作包在 select 里，别裸 `<-ch`
- HTTP client 用 `http.Client{Timeout: ...}`，别用 `http.DefaultClient`（无超时）
- `sync.WaitGroup` 确保 `defer wg.Done()`，别在可能有 early return 的路径漏调

---

## 二、运行时死锁（A 类，进程猝死）

 ### 现象

 进程没了。重启后看日志，最后一行是这个：

```
fatal error: all goroutines are asleep - deadlock!

goroutine 1 [chan receive (no senders)]:
main.main()
    /app/main.go:50 +0x200

goroutine 2 [chan send (no receivers)]:
main.producer(...)
    /app/main.go:12 +0x88
created by main.main in goroutine 1
    /app/main.go:20 +0x100
...
```

### 谁检测的

 **💡 D2 锚点 — 这是 Go runtime 自己干的，不是你的代码：** runtime 的调度器每次
 调度循环结束时检查"还有没有可运行的 goroutine"。如果**所有** goroutine 都阻塞、
 且没有任何外部事件能让它们醒来（无网络 IO、无 timer、无系统调用返回），
 runtime 判定整死，直接 `panic("deadlock")`。

 所以这个 panic **抓不到 profile**——进程当场崩，pprof 没了。排查**全靠日志里的堆栈**。
 这也是为什么这个 slice 不搭 lab：搭了也是进程一启动就崩，演示价值有限。

### 怎么读崩溃堆栈

 日志会列出**所有** goroutine 的状态。找互相等待的**环**：

```
goroutine A [持有 lock1，等 lock2]  ← A 卡在拿 lock2
goroutine B [持有 lock2，等 lock1]  ← B 卡在拿 lock1
```

 A 等 B 释放 lock2，B 等 A 释放 lock1，谁也不让 → 死锁。

 三种经典形态：
1. **mutex 交叉加锁顺序不一致**：A 先锁 m1 再锁 m2，B 先锁 m2 再锁 m1。修法：全局统一加锁顺序。
2. **channel 双向等待**：goroutine A `ch <- x` 等接收方，但唯一可能的接收方 goroutine B 正在等 A 先发别的数据。修法：用 buffered channel 或拆成两个独立 channel。
3. **WaitGroup 死结**：在持有 wg 的 goroutine 里 `wg.Wait()` 又在同 goroutine 里期待 `wg.Done()`，自己等自己。修法：fork 和 wait 分开。

### 假死锁：不是所有 "all asleep" 都是真死锁

 有时候你会看到这个错误但其实**不是代码 bug**：
- **启动时机问题**：程序刚起来，main goroutine 在等某个后台 goroutine 启动完成，但那个 goroutine 因为 init 顺序没起来。常见于测试。
- **cgo / 系统调用**：goroutine 阻塞在 cgo 调用里，runtime 把它算作"不运行"，但其实不是死锁。

 区分：看堆栈里有没有卡在 `runtime.cgocall` 或明确的 cgo 函数，有的话先查 cgo。

---

## 三、和前两个 slice 的关系

 排查"进程出问题"的脑回路，按现象分流：

```text
进程还活着？
├─ 是 → 看资源指标
│       ├─ CPU 高 → [cpu-hotspot.md] 抓 CPU profile
│       ├─ 内存涨 → [memory-leak.md] 抓 heap profile
│       └─ goroutine 涨 → 本文档（一）：抓 goroutine profile
└─ 否（崩了）→ 看崩溃日志最后一行
        ├─ "all goroutines are asleep - deadlock!" → 本文档（二）：读堆栈找环
        ├─ "fatal error: runtime: out of memory" → GOMEMLIMIT / 容器内存不足
        └─ "signal: killed" → OOMKill（被系统杀），看 dmesg / kubectl describe
```

 这个分流树记住了，遇到事故第一秒就知道往哪走。

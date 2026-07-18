# CPU 热点排查（Troubleshooting Lab）

> 这是"线上 CPU 排查"的实操 lab。我们在 `order-api` 的创建订单热路径里注入了一个
> 典型的线上 bug（热路径里的 O(n²) 低效实现），用真实流量打满 CPU，再用 pprof 把它抓出来。
> 默认关闭，只在 `BUG_CPU=1` 时生效。

## 这个 lab 的故障长什么样

[lab_faults.go](../../service/order/api/internal/logic/order/lab_faults.go) 里的 `computeRiskScore`
模拟"反欺诈风控评分"：对商品名做重复字符特征扫描。这是线上 CPU bug 最常见的形态之一：

- 在**请求热路径**里做了 O(n²) 的计算
- 单次请求耗 CPU 几十~几百毫秒
- 常规压测流量小，根本看不出来；真实流量一上来 CPU 立刻打满

调用点在 [createOrderLogic.go](../../service/order/api/internal/logic/order/createOrderLogic.go) 的 `Create()` 最前面，
只有 `BUG_CPU=1` 时才触发。

**为什么不用正则 ReDoS：** Go 的 `regexp` 是 RE2 引擎，对回溯类 ReDoS 免疫。
想用正则烧 CPU 在 Go 里行不通——这一点本身也是排查时的一个判断依据（见下方 D2）。

---

## Step 0：触发故障 + 观察告警

### 启动带故障的服务

```bash
# 在一个长 session（PTY）里启动，别用 nohup &——会话结束会被回收
BUG_CPU=1 BUG_LOAD_RATE=100000 go run ./service/order/api -f service/order/api/etc/order-api.yaml
```

两个环境变量缺一不可，原因见最后的 [踩坑](#踩坑为什么-load-打不上去)。

### 触发告警

对应的告警规则在 [rules.yml](../../deploy/prometheus/rules.yml) 的 `runtime_alerts` 组：

```yaml
- alert: HighCPUUsage
  expr: rate(process_cpu_seconds_total[5m]) > 0.8
  for: 5m
```

观察 CPU 占用（值就是占用的 CPU **核数**，不是百分比）：

```bash
curl -s --data-urlencode 'query=rate(process_cpu_seconds_total{job="order-api"}[2m])' \
  "http://localhost:9091/api/v1/query" | python3 -m json.tool
```

**💡 D2 锚点 — `rate(process_cpu_seconds_total[5m])` 的值为什么是核数：**
`process_cpu_seconds_total` 是进程累计消耗的 CPU 秒数（含所有核）。
`rate(...[5m])` 是"每秒消耗的 CPU 秒数"。每秒消耗 1 秒 CPU = 用满 1 个核。
所以 `> 0.8` 就是"持续占用超过 0.8 个核"。多核机器上这个值可以大于 1（本 lab 实测能到 4+）。

### 压测

```bash
TOKEN=$(cat /tmp/jwt.txt)
for w in $(seq 1 8); do
  (
    END=$((SECONDS+360)); i=0
    while [ $SECONDS -lt $END ]; do
      i=$((i+1))
      curl -s -o /dev/null -X POST http://localhost:8081/api/v1/orders \
        -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
        -H "Idempotency-Key: f${w}-$i-$(date +%s%N)" \
        -d '{"product_name":"Premium Widgets Pro Max Ultra Series X-2026 Special Edition Pack","sku":"SKU-CPU-001","quantity":1,"amount":100}'
    done
  ) &
done
wait
```

8 个 worker 并发打 6 分钟，CPU 能压到 4+ 核。

---

## Step 1：抓 30 秒 CPU profile

```bash
go tool pprof -seconds=30 http://localhost:6060/debug/pprof/profile
```

进入 pprof 交互式 shell。也可以直接存盘：

```bash
curl -o /tmp/lab-profiles/cpu-before.prof \
  "http://localhost:6060/debug/pprof/profile?seconds=30"
```

**💡 D2 锚点 — 为什么是 30 秒：** Go 的 CPU profile 是**采样**的，默认 100Hz（每秒 100 次）。
30 秒 ≈ 3000 个采样点。采样太少（< 1s）噪声大，采样太长（> 2min）期间代码可能已变化。
30s 是线上抓 profile 的甜点：既能覆盖到足够的请求，又不会拖太久。

**💡 D2 锚点 — profile 抓的是"在 CPU 上执行的栈"：** 每次采样记录"此刻这个 goroutine 正在
CPU 上跑哪段代码"。所以 GC、系统调用、用户代码都会被抓到。卡在 IO 等待（不占 CPU）的
goroutine **不会**出现在 CPU profile 里——它们要去 goroutine profile 找。

---

## Step 2：`top -cum` 找热点

在 pprof shell 里：

```
(pprof) top -cum
```

本 lab 的真实输出：

```
Showing nodes accounting for 122.51s, 99.37% of 123.29s total
      flat  flat%   sum%        cum   cum%
         0     0%     0%    122.47s 99.33%  net/http.HandlerFunc.ServeHTTP
         ...
         0     0%     0%    122.05s 98.99%  ...CreateOrderLogic).Create
   111.41s 90.36% 90.36%    117.86s 95.60%  ...logic/order.computeRiskScore  ← 热点
     6.45s  5.23% 95.60%      6.45s  5.23%  runtime.asyncPreempt
     4.65s  3.77% 99.37%      4.65s  3.77%  syscall.rawsyscalln
```

一眼看到 `computeRiskScore`：flat 90.36%，cum 95.60%。整个进程几乎一半的 CPU 时间花在这个函数里。

**💡 D2 锚点 — flat vs cum：**
- **flat** = 函数**自身**代码消耗的 CPU（不含它调用的子函数）
- **cum** (cumulative) = 函数自身 + 它调用的所有子函数消耗的 CPU

`computeRiskScore` flat=90% cum=95% 说明：热点就在这个函数自己的循环里，不是它调用的别的东西。
反过来如果某函数 flat≈0 但 cum 很高，说明它只是个"中转站"，真正的热点在它下游——继续往下追。

**💡 D2 锚点 — `Total samples = 123.29s (409.09%)` 是什么：** 30 秒里抓到的总采样时间
是 123 秒，因为期间程序同时在 **4 个 CPU 核**上跑（30s × 4核 ≈ 120s）。`409.09%` 就是"用了
4.09 个核"。

---

## Step 3：`list` 进函数看具体代码行

```
(pprof) list computeRiskScore
```

输出会标注每一行消耗的 CPU 时间：

```
ROUTINE ======================== ...computeRiskScore in .../lab_faults.go
   111.41s   117.86s (flat, cum) 90.36% of Total
       .          .    42:   for k := 0; k < iterations; k++ {
       .          .    43:       n := len(name)
       .          .    44:       for i := 0; i < n; i++ {
       .          .    45:           for j := i + 1; j < n; j++ {
   111.41s    117.86s    46:               if name[i] == name[j] {
       .          .    47:                   score++
       .          .    48:               }
```

第 46 行（双层循环的比较）独占 111 秒。定位到这一行，bug 的根因就清楚了：
外层 `iterations=50000` 把一次 O(n²) 放大了五万次。

---

## Step 4：火焰图（最直观）

```bash
go tool pprof -http=:8080 /tmp/lab-profiles/cpu-before.prof
```

浏览器打开 `http://localhost:8080` → FLAME GRAPH。

**怎么看火焰图：**
- 横轴 = CPU 时间占比，纵轴 = 调用栈（下在上之上）
- **最宽的"平顶"**就是热点：一块很宽的色块说明大量 CPU 时间花在这一个函数上
- 本 lab 的火焰图里，`computeRiskScore` 那一段会是一个巨大的平顶，几乎占满整张图

火焰图比 `top` 更适合**给不熟悉代码的人**讲清楚"问题出在哪"，是线上事故复盘的标准材料。

---

## Step 5：判断是业务代码还是 GC

同一个 profile 的 `top` 里，注意这几行：

```
   111.41s 90.36%  ...  computeRiskScore   ← 业务热点
     6.45s  5.23%  ...  runtime.asyncPreempt
     4.65s  3.77%  ...  syscall.rawsyscalln
```

**💡 D2 锚点 — 看见这些 runtime 函数意味着什么：**
- `runtime.gcBgMarkWorker` / `runtime.mallocgc` 占比高 → **GC 压力**。根因是分配过多对象
  （去查 heap profile 的 `alloc_space`）。本 lab 没有这个，因为 `computeRiskScore` 0 allocs。
- `runtime.asyncPreempt` 高 → goroutine 被频繁抢占，通常是 goroutine 太多或锁竞争激烈
  导致调度抖动（本 lab 是因为 8 worker 并发，属正常伴生）。
- `syscall.*` 高 → IO 密集或系统调用过多，去看是不是有未加 buffer 的 IO 循环。

本 lab 的结论很干净：90% 是纯业务热点，修业务代码即可。

---

## 修复方向

`computeRiskScore` 是 O(n²) × iterations。真实场景的修法：

1. **改算法**：用 map 统计字符频次，一次 O(n) 就能算出重复对数，去掉外层 50000 次放大
2. **挪出热路径**：风控评分本就该异步，发到 Kafka 由单独的 risk worker 消费，下单只返回
3. **降级开关**：保留同步路径但用很小的 iterations（如 100）作为兜底，真实评分走异步

修完重新抓一份 `cpu-after.prof`，和 `before` 对比：

```bash
go tool pprof -base /tmp/lab-profiles/cpu-before.prof /tmp/lab-profiles/cpu-after.prof
```

`-base` 会显示 after 相对 before 的**变化**，确认热点消失。

---

## 验证（V1 + V2）

- **V1（metrics 恢复）**：Prometheus 里 `rate(process_cpu_seconds_total{job="order-api"}[5m])`
  回落到 < 0.8 核；`HighCPUUsage` 告警消失
- **V2（profile diff）**：`computeRiskScore` 从 90% flat 降到接近 0

---

## 踩坑：为什么 load 打不上去

本 lab 的故障在 `Create()` 最前面，但要到达它，请求得过三道关。这三道关也是线上排查
CPU 问题时常见的"流量到不了热点"的原因：

1. **go-zero 熔断中间件**：`user-rpc` 没起来时，breaker 会打开，直接拒绝请求。
   [order-api.yaml](../../service/order/api/etc/order-api.yaml) 里设 `Middlewares: Breaker: false` 绕过。
2. **Redis 限流器**：默认 100 次/分钟/IP，压测流量直接被限。
   [serviceContext.go](../../service/order/api/internal/svc/serviceContext.go) 用 `BUG_LOAD_RATE` 环境变量调高（设 100000）。
3. **后台 `nohup &` 进程会被回收**：exec 会话结束时，子进程一起被收掉。
   必须用一个长 session（PTY）或前台跑。

**💡 D2 锚点 — 告警停在 Pending 没变成 Firing：** `HighCPUUsage` 有 `for: 5m`。
如果压测只跑了 2 分钟，CPU 虽然打满了，但告警条件"连续 5 分钟满足"没达到，
状态会停在 `pending` 而不是 `firing`。这是 Prometheus 的正常行为，不是 bug。
线上遇到 Pending 也值得看——它意味着"快出问题了，但还没持续够时间"。

---

## 命令速查

| 目的 | 命令 |
|------|------|
| 看 CPU 核数 | `rate(process_cpu_seconds_total[5m])` |
| 抓 30s profile（交互） | `go tool pprof -seconds=30 http://localhost:6060/debug/pprof/profile` |
| 抓 30s profile（存盘） | `curl -o before.prof "http://localhost:6060/debug/pprof/profile?seconds=30"` |
| 热点排序 | `(pprof) top -cum` |
| 看函数代码行 | `(pprof) list <函数名>` |
| 调用图 | `(pprof) web`（需 graphviz） |
| 火焰图 Web UI | `go tool pprof -http=:8080 <prof>` |
| 对比 before/after | `go tool pprof -base before.prof after.prof` |

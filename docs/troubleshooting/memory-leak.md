# 内存泄漏排查（Troubleshooting Lab）

> 这是"线上内存泄漏排查"的实操 lab。我们在 `order-api` 里注入了一个全局"风控画像缓存"，
> 只进不出、永不淘汰——这是线上内存泄漏最高频的形态。用真实流量填充，再用 heap profile 把它抓出来。
> 默认关闭，只在 `BUG_MEMLEAK=1` 时生效。

## 这个 lab 的故障长什么样

[lab_faults.go](../../service/order/api/internal/logic/order/lab_faults.go) 里的 `cacheRiskProfile`
把每条请求的"风控画像"塞进一个**全局 map**，key 是 idempotency key（每请求唯一），value 带一个 32KB 的 blob。
没有任何淘汰路径，堆只增不减。

调用点在 [createOrderLogic.go](../../service/order/api/internal/logic/order/createOrderLogic.go) 的 `Create()` 里，`BUG_MEMLEAK=1` 时触发。

**为什么选"缓存带 blob"这个形态：** 线上内存泄漏最常见的不是"忘 free 一个大数组"，而是
"缓存对象本身很小，但 value 里顺手存了原始 payload / DB 行 / 缩略图 / HTTP response body"。
单看缓存条目数觉得没什么，实际每条吃几十 KB，条目一多堆就爆。这个 lab 的 `riskProfile.Snapshot`
就是模拟这个 blob。

---

## Step 0：触发故障 + 观察告警

### 启动

```bash
BUG_MEMLEAK=1 BUG_LOAD_RATE=100000 go run ./service/order/api -f service/order/api/etc/order-api.yaml
```

### 告警规则

[rules.yml](../../deploy/prometheus/rules.yml) 的 `runtime_alerts` 组里有两条堆告警，互补使用：

```yaml
- alert: HeapUsageHigh         # 绝对水位
  expr: go_memstats_heap_inuse_bytes > 536870912   # 512 MiB
  for: 5m
- alert: HeapLeakSuspected      # 相对增速（早期信号）
  expr: go_memstats_heap_inuse_bytes > 2 * (go_memstats_heap_inuse_bytes offset 10m)
  for: 3m
```

观察当前堆（`go_memstats_heap_inuse_bytes` 是**当前存活**的字节数，不含已 GC 的）：

```bash
curl -s --data-urlencode 'query=go_memstats_heap_inuse_bytes{job="order-api"}' \
  "http://localhost:9091/api/v1/query" | python3 -c "
import sys,json
d=json.load(sys.stdin)['data']['result']
print(f\"{int(d[0]['value'][1])/1024/1024:.1f} MiB\" if d else 'no data')
"
```

**💡 D2 锚点 — 两条告警各抓什么：**
- `HeapUsageHigh` 看绝对水位，告诉你"已经太高了"——但等到它触发，问题已经很严重。
- `HeapLeakSuspected` 看**相对增速**（10 分钟翻倍），是泄漏的**早期信号**，在绝对水位还不高时就能报警。
线上两条都要有：前者兜底，后者预警。

### 先抓一份 baseline（空载）

```bash
curl -o /tmp/lab-profiles/mem-baseline.prof http://localhost:6060/debug/pprof/heap
```

**💡 D2 锚点 — 排查内存泄漏一定要有 baseline：** 单看一个 heap profile 只知道"现在堆里有什么"，
分不清"正常该有的"和"泄漏的"。抓一份故障前的 baseline，事后用 `-base` 做 diff，**只看增长部分**——
那才是泄漏对象。这是 V2 验证法的核心。

### 压测填充缓存

```bash
TOKEN=$(cat /tmp/jwt.txt)
for w in $(seq 1 8); do
  (
    END=$((SECONDS+300)); i=0
    while [ $SECONDS -lt $END ]; do
      i=$((i+1))
      curl -s -o /dev/null -X POST http://localhost:8081/api/v1/orders \
        -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
        -H "Idempotency-Key: m${w}-$i-$(date +%s%N)" \
        -d '{"product_name":"Premium Widgets","sku":"SKU-MEM-001","quantity":1,"amount":100}'
    done
  ) &
done
wait
```

实测：8 worker 跑约 90 秒，堆从 7 MiB 涨到 1.26 GiB，两条告警先后进 pending。

---

## Step 1：抓 heap profile（在堆高位时）

```bash
curl -o /tmp/lab-profiles/mem-leaking.prof http://localhost:6060/debug/pprof/heap
```

**💡 D2 锚点 — heap profile 默认抓 `inuse_space`：** `/debug/pprof/heap` 默认返回的是
**当前存活的分配**（inuse），不是累计分配（alloc）。这对抓泄漏很关键——inuse 里还在的对象，
就是"GC 没能回收的"，正是泄漏候选。

**💡 D2 锚点 — 什么时候抓：** 在堆高位、且确认还在增长时抓，最容易看出来。
如果堆已经回落（比如恰好 GC 了或流量停了），抓到的可能不准。配合 `HeapLeakSuspected`
的增速曲线，挑"还在涨"的瞬间抓。

---

## Step 2：`top` 看谁占的内存最多

```bash
go tool pprof -top -inuse_space /tmp/lab-profiles/mem-leaking.prof
```

本 lab 真实输出：

```
Showing nodes accounting for 716.91MB, 97.92% of 732.11MB total
      flat  flat%   sum%        cum   cum%
  716.91MB 97.92% 97.92%   716.91MB 97.92%  ...cacheRiskProfile  ← 泄漏点
         0     0% 97.92%   716.91MB 97.92%  ...(*CreateOrderLogic).Create
         0     0% 97.92%   717.42MB 97.92%  ...(*RedisRateLimiter).Middleware.func1
```

一个函数独占 716 MB / 97.92%。调用栈 `Create → cacheRiskProfile` 一目了然。

heap profile 的 flat/cum 含义和 CPU profile 一样，但单位是**字节**：
- **flat** = 这个函数自己的代码分配的、且**当前还存活**的字节
- **cum** = 这个函数 + 它调用的子函数分配的、且当前还存活的字节

`cacheRiskProfile` flat≈cum≈716MB，说明字节是在它**自己**里分配的（`make([]byte, 32*1024)`），
不是下游分配的。

---

## Step 3：三个 sample 维度，判断泄漏类型

heap profile 可以切换四种 sample，排查时要**对比看**：

```bash
go tool pprof -top -inuse_space    mem-leaking.prof   # 当前存活的字节
go tool pprof -top -inuse_objects  mem-leaking.prof   # 当前存活的对象数
go tool pprof -top -alloc_space    mem-leaking.prof   # 累计分配的字节
go tool pprof -top -alloc_objects  mem-leaking.prof   # 累计分配的对象数
```

本 lab 的对比（真实数据）：

| sample | 第一名 | 解读 |
|--------|--------|------|
| inuse_space | `cacheRiskProfile` 716MB (98%) | 当前堆里几乎全是它 → 泄漏确认 |
| alloc_space | `cacheRiskProfile` 717MB (40%) | 累计分配里它最大，但占比没 inuse 高（因为有大量正常的临时分配） |
| alloc_objects | otel/reflect 各种 (各 ~5%) | 对象**数**上它排不上号——因为它分配的是少数大对象，不是海量小对象 |
| inuse_objects | （同理） | 存活对象数不突出 |

**💡 D2 锚点 — 这个对比能告诉你泄漏的"形状"：**
- **inuse_space 涨，但 alloc_objects 不涨** → 少数**大对象**泄漏（本 lab 这种：每条 32KB blob）。
  根因通常是缓存了带 blob 的值。
- **inuse_objects 涨，alloc_space 不突出** → 海量**小对象**泄漏。
  根因通常是 map entry / slice element 没删、迭代器没关、defer 在循环里累积。
- **alloc 很高但 inuse 不高** → 分配压力大但能回收，不是泄漏，是 GC 压力问题（去优化分配）。

这个判断直接决定你去 grep 什么样的代码：大对象泄漏找"缓存/持有 blob 的地方"，
小对象泄漏找"循环里 append / map 赋值但没清理的地方"。

---

## Step 4：用 `-base` diff，只看增长部分

```bash
go tool pprof -base /tmp/lab-profiles/mem-baseline.prof /tmp/lab-profiles/mem-leaking.prof
```

在 pprof shell 里 `top`：

```
Showing nodes accounting for 716.91MB, 98.60% of 727.09MB total
      flat  flat%   sum%        cum   cum%
  716.91MB 98.60% 98.60%   716.91MB 98.60%  ...cacheRiskProfile
```

diff 后显示的是 **leaking 相对 baseline 的增量**。本 lab 的增量 100% 是 `cacheRiskProfile`，
极其干净。真实线上通常不会这么干净（堆里混着各种正常对象），但 diff 能把噪声滤掉，让泄漏对象凸显。

**💡 D2 锚点 — `gc` 之后抓更准：** 如果想确认"这些字节真的不会被回收"，可以在抓 profile 前
先触发一次 GC（或调 `/debug/pprof/heap?gc=1`，pprof 抓之前会先 GC）。GC 后还在的字节，
就是被引用着、回收不掉的——比纯泄漏。

---

## Step 5：火焰图看分配路径

```bash
go tool pprof -http=:8080 -inuse_space /tmp/lab-profiles/mem-leaking.prof
```

FLAME GRAPH 里找最宽的色块，本 lab 是 `cacheRiskProfile`，它下面是 `Create`，再下面是 HTTP 中间件链。
宽平顶 = 这个函数自己分配了大量内存。火焰图的好处是能一眼看到**完整的分配调用路径**，
方便在代码里顺藤摸瓜。

---

## 修复方向

这个 lab 的 `riskProfileCache` 是个无淘汰的全局 map。真实修复：

1. **加上限 + TTL**：换成 `freecache` / `bigcache` / `ristretto` 这类自带淘汰的库，
   或自己用 `map + 过期检查`。设 `MaxEntries` 和 per-entry TTL。
2. **别缓存 blob**：`riskProfile` 里只缓存真正需要的字段（UserID/Score），不要把 `Snapshot` 也存进去。
   这是最直接的修复——泄漏的根因是"缓存了不该缓存的大字段"。
3. **如果必须缓存大对象**：考虑用磁盘/Redis 外部存储，本地只放指针或索引。

修复后重新抓 `mem-after.prof`，和 `mem-leaking.prof` diff，确认 `cacheRiskProfile` 的 inuse 归零。

---

## 验证（V1 + V2）

- **V1（metrics 恢复）**：`go_memstats_heap_inuse_bytes` 回落并稳定在低位；
  `HeapUsageHigh` / `HeapLeakSuspected` 消失
- **V2（profile diff）**：`mem-after.prof` 相对 `mem-baseline.prof` 增量里，
  `cacheRiskProfile` 不再出现

---

## 内存泄漏 vs GC 压力：别搞混

这两个问题都会让"内存相关指标异常"，但根因和修法完全不同：

| | 内存泄漏 | GC 压力 |
|--|----------|---------|
| inuse_space | 持续单调上涨，不回落 | 波动，但 GC 后能回落 |
| alloc_space | 正常 | 极高（疯狂分配） |
| CPU profile | GC 函数占比正常 | `runtime.gcBgMarkWorker`/`runtime.mallocgc` 占比高 |
| 修法 | 找泄漏引用，断开它 | 减少分配（对象池/复用/减少临时对象） |
| 告警 | HeapLeakSuspected | HighCPUUsage（GC 烧 CPU）|

**💡 D2 锚点 — `runtime.gcBgMarkWorker` 高 ≠ 内存泄漏：** 它高说明 GC 在拼命干活，
通常是因为**分配太快**（GC 压力），不是因为内存泄漏。泄漏的对象 GC 根本碰不到（还被引用着），
反而可能让 GC 看起来"轻松"（因为能回收的少了）。判断泄漏看 `inuse_space` 单调性，不看 GC 占比。

---

## 命令速查

| 目的 | 命令 |
|------|------|
| 看当前堆 | `go_memstats_heap_inuse_bytes` |
| 抓 heap（默认 inuse） | `curl -o heap.prof http://localhost:6060/debug/pprof/heap` |
| 抓 heap（先 GC） | `curl -o heap.prof "http://localhost:6060/debug/pprof/heap?gc=1"` |
| 当前存活字节 | `go tool pprof -top -inuse_space heap.prof` |
| 当前存活对象数 | `go tool pprof -top -inuse_objects heap.prof` |
| 累计分配字节 | `go tool pprof -top -alloc_space heap.prof` |
| 累计分配对象数 | `go tool pprof -top -alloc_objects heap.prof` |
| diff（只看增长） | `go tool pprof -base baseline.prof leaking.prof` |
| 火焰图 | `go tool pprof -http=:8080 -inuse_space heap.prof` |
| list 看代码行 | `(pprof) list <函数名>` |

# pprof 学习指南

> 这份文档从零讲 Go 的 pprof：它是什么、怎么采集、怎么读、怎么在三类常见问题
> （CPU 热点 / 内存问题 / goroutine 问题）里用好它。读完你能回答这些：
> - CPU profile 和 heap profile 抓的东西有什么本质区别？
> - `flat` 和 `cum` 到底是什么？什么时候看哪个？
> - heap 的 `inuse_space` / `inuse_objects` / `alloc_space` / `alloc_objects` 各代表什么，什么时候用哪个？
> - goroutine profile 的 `debug=1` 和 `debug=2` 抓出来长什么样，怎么读？
>
> 本文档是「学习/原理」视角。想直接动手排查线上事故，去 [troubleshooting/](troubleshooting/) 走三个 lab：
> [CPU 热点](troubleshooting/cpu-hotspot.md) / [内存泄漏](troubleshooting/memory-leak.md) /
> [Goroutine 泄漏与死锁](troubleshooting/goroutine-deadlock-quickref.md)。

## 目录

- [一、pprof 是什么](#一pprof-是什么)
- [二、两种采集方式：`net/http/pprof` vs `runtime/pprof`](#二两种采集方式nethttppprof-vs-runtimepprof)
- [三、核心概念：采样、快照、flat、cum、sample 类型](#三核心概念采样快照flatcumsample-类型)
- [四、工具链：`go tool pprof` 用法全解](#四工具链go-tool-pprof-用法全解)
- [五、在 go-zero 服务里启用 pprof](#五在-go-zero-服务里启用-pprof)
- [六、CPU profile：原理、采集、分析](#六cpu-profile原理采集分析)
- [七、heap profile：原理、四种维度、泄漏判断](#七heap-profile原理四种维度泄漏判断)
- [八、goroutine profile：原理、debug 模式、卡点读法](#八goroutine-profile原理debug-模式卡点读法)
- [九、生产环境的注意事项](#九生产环境的注意事项)
- [十、命令速查总表](#十命令速查总表)
- [十一、学习路径建议](#十一学习路径建议)

---

## 一、pprof 是什么

pprof 是 Go runtime 自带的性能分析工具，回答一个问题：**这个 Go 程序的资源（CPU 时间、内存、goroutine、锁、线程、阻塞）都花在哪里了？**

它的能力分两层：

| 层 | 提供方 | 作用 |
|----|--------|------|
| **采集层** | `runtime/pprof` 和 `net/http/pprof` 两个标准库 | 在 Go 程序里收集 profile 数据（栈 + 采样值） |
| **分析层** | `go tool pprof` 命令（Go 工具链自带） | 读 profile、排序、画火焰图、diff |

这两层是分离的。你的 Go 程序只负责"采"——把数据以 protobuf 格式暴露出来；分析全在 `go tool pprof` 里做，它既能读本地文件，也能直接从 HTTP 拉取。

### 1.1 pprof 能干什么、不能干什么

**能干：**

- 找 CPU 热点（哪个函数占 CPU 最多）
- 找内存分配大户、找内存泄漏
- 找 goroutine 泄漏、死锁卡点
- 看锁竞争（`mutex` profile）、阻塞时长（`block` profile）、线程创建（`threadcreate`）

**不能干（或不是它的强项）：**

- **不能用来看"为什么慢"如果慢在网络/磁盘 IO 等待**。CPU profile 只记"在 CPU 上跑的栈"，
  卡在 `read(socket)` 等数据的 goroutine 不会出现在 CPU profile 里——它们在 goroutine profile 里
  状态是 `[IO wait]`。
- **不能做细粒度的单请求 trace**。它采样的是统计意义上的分布，不是"某一次请求发生了什么"。
  想看单次请求的时间线用 `go tool trace`（runtime/trace）。
- **不能调试逻辑 bug / 数据错误**。那是 dlv（`go-delve/delve`）和日志的活。

**💡 D2 锚点 — pprof vs dlv vs trace 怎么选：**

| 工具 | 看什么 | 颗粒度 | 开销 |
|------|--------|--------|------|
| **pprof** | 资源（CPU/内存/goroutine）花在哪 | 函数级，统计采样 | 低（CPU 100Hz 采样，内存默认 1/512 采样） |
| **trace** (`runtime/trace`) | 事件时间线、调度、GC、单请求链路 | 事件级，全量记录 | 高，只用在短时间定位 |
| **dlv** (`dlv debug`) | 逻辑 bug、断点、变量、调用栈 | 单步精确 | 暂停式，不能在线上开 |

线上"慢"、"涨"、"卡"找 pprof；时间线和调度找 trace；逻辑错找 dlv。

### 1.2 pprof 能抓的 profile 类型

`/debug/pprof/` 下默认暴露这些（部分需要显式开启）：

| endpoint | 抓什么 | 默认是否开启 |
|----------|--------|-------------|
| `/debug/pprof/profile` | CPU profile（一段时间内的 CPU 采样） | 是 |
| `/debug/pprof/heap` | 堆分配 profile | 是 |
| `/debug/pprof/goroutine` | 当前所有 goroutine 的栈 | 是 |
| `/debug/pprof/threadcreate` | OS 线程创建栈 | 是 |
| `/debug/pprof/allocs` | 堆分配（含历史，和 heap 同源，sample 类型不同） | 是 |
| `/debug/pprof/block` | 阻塞事件（chan / mutex / select 等） | **否**，需 `runtime.SetBlockProfileRate` |
| `/debug/pprof/mutex` | 锁竞争事件 | **否**，需 `runtime.SetMutexProfileFraction` |

本指南聚焦你最常用的三个：`profile`（CPU）、`heap`（内存）、`goroutine`。block 和 mutex
属于进阶，文末提一下。

---

## 二、两种采集方式：`net/http/pprof` vs `runtime/pprof`

Go 给了两个采集入口，分别对应"长跑的线上服务"和"短跑的批处理/测试"。

### 2.1 `net/http/pprof`：服务在线上时用

只要 import 一下，它的 `init()` 会自动把 `/debug/pprof/*` 路由注册到 `http.DefaultServeMux`：

```go
import _ "net/http/pprof"

func main() {
    // 你的业务 server 用自己的 mux，另起一个只在内网监听的 HTTP server 跑 pprof
    go func() {
        log.Println(http.ListenAndServe("127.0.0.1:6060", nil))
    }()
    // ... 业务逻辑
}
```

两个要点：

1. **`import _ "net/http/pprof"`**：下划线表示只跑它的 `init()`，不直接调用——init 里会
   往 `http.DefaultServeMux` 注册 `/debug/pprof/` 下的所有 handler。
2. **独立 server、独立端口、只绑内网**：不要把 pprof 挂到业务的 mux 上，更不要暴露公网
   （见 [九、生产注意事项](#九生产环境的注意事项)）。

然后你就能 `curl http://localhost:6060/debug/pprof/` 看到 endpoint 列表，所有 profile
通过 HTTP 拉，**不用停服务**。

**💡 D2 锚点 — 为什么强调"独立 mux、独立端口"：** pprof 的 handler 有性能开销，挂到业务
路由上会影响真实请求；而且它们暴露完整的调用栈和内存布局，等于把代码结构白送攻击者。
绑内网、独立端口、（必要时）加 Basic Auth，是基本卫生。

### 2.2 `runtime/pprof`：批处理、benchmark、一次性程序用

没有 HTTP server 的程序（CLI、批处理 job、benchmark），用 `runtime/pprof` 把 profile 写文件：

```go
import "runtime/pprof"

func main() {
    f, _ := os.Create("cpu.prof")
    pprof.StartCPUProfile(f)   // 开始采样
    defer pprof.StopCPUProfile() // 程序退出前结束，flush 到文件

    // ... 你要 profile 的代码
}
```

然后 `go tool pprof cpu.prof` 分析。

benchmark 不用写代码，直接用 `go test` 的 `-cpuprofile` / `-memprofile` flag：

```bash
go test -bench=. -cpuprofile=cpu.prof -memprofile=mem.prof ./...
```

**💡 D2 锚点 — benchmark 的 profile 和线上 profile 有什么不一样：** 数据格式完全一样。
区别是 benchmark 的负载是受控的（你写的 B 个迭代），所以噪声小，适合做"A vs B"的微优化
对比。线上 profile 噪声大（流量、GC、调度都不可控），但反映真实分布。

### 2.3 什么时候用哪个

| 场景 | 用哪个 |
|------|--------|
| 长跑的 HTTP/gRPC 服务（线上） | `net/http/pprof` |
| go-zero 等框架自带 pprof endpoint | 直接用框架的（见 [五](#五在-go-zero-服务里启用-pprof)） |
| CLI 工具、批处理 job | `runtime/pprof` 写文件 |
| 单元测试、benchmark | `go test -cpuprofile` / `-memprofile` |
| 想在特定代码段采样（不受 HTTP 控制） | `runtime/pprof` + `StartCPUProfile` / `pprof.WriteHeapProfile` |

---

## 三、核心概念：采样、快照、flat、cum、sample 类型

这部分是 pprof 最容易卡住的地方。理解了这几个概念，剩下的全是套路。

### 3.1 采样 vs 快照

不同 profile 的"采集方式"不一样，这直接决定了你怎么读它：

| profile | 采集方式 | 含义 |
|---------|----------|------|
| **CPU** (`/debug/pprof/profile`) | **基于时间的采样**：默认每 10ms 触发一次信号（SIGPROF），记录所有正在 CPU 上跑的 goroutine 的当前栈。30 秒 ≈ 3000 个样本。 | 统计意义上的"CPU 时间分布" |
| **heap** (`/debug/pprof/heap`) | **基于分配事件的采样**：每分配 `1/rate` 字节记录一次（默认 `rate=runtime.MemProfileRate=512KB`）。抓取瞬间 dump 当前累计的样本。 | "分配发生过哪些栈"+"当前还存活多少" |
| **goroutine** (`/debug/pprof/goroutine`) | **瞬时快照**：调用 `runtime.Stack(全部 goroutine)`，一次性抓当前所有 goroutine 的栈。 | "此刻每个 goroutine 在干什么" |

**💡 D2 锚点 — 这个区别为什么重要：**

- CPU profile 必须**跑一段时间**（默认 30s）才有意义，因为它采的是"频率"。
- heap profile 和 goroutine profile 是**瞬时快照**，访问一次 URL 立刻返回。
  这意味着它们只反映"抓的那一刻"——多抓几次对比，才有意义。
- 这也解释了为什么内存泄漏排查强调"先抓 baseline、再抓 leaking、做 diff"：
  单看一个快照分不清"正常的"和"泄漏的"。

### 3.2 `flat` 和 `cum`：必须分清的两个数

每一个函数在 profile 里都有两个数：

- **`flat`** = 这个函数**自己**代码消耗的资源（不含它调用的子函数）
- **`cum`**（cumulative）= 这个函数自己 + 它**调用的所有子函数**消耗的资源

举例：函数 A 调用 B，B 调用 C。

```text
A (flat=0,   cum=100)   ← A 自己没干活，但它的子调用累计花了 100
└─ B (flat=10, cum=100)  ← B 自己花了 10，加上 C 的 90 = 100
   └─ C (flat=90, cum=90) ← C 自己花了 90，没调用别人
```

**怎么用：**

- **找"热点本身"**：按 `flat` 排序，flat 最高的函数就是"自己最耗资源"的——通常是 bug 所在。
- **追"调用链"**：看 `cum`，从 `main` 一路往下，cum 高的链就是资源流向。
- **`flat≈0 但 cum 很高`**：函数只是个"中转"（dispatch、middleware），真正的热点在它下游，继续往下追。

CPU profile 里 flat/cum 的单位是**CPU 时间**（秒）；heap profile 里单位是**字节**或**对象数**。

### 3.3 sample 类型：heap 的四种维度

heap profile 有四种 sample 类型，**用同一个 `.prof` 文件切换即可**：

| sample | 含义 | 用途 |
|--------|------|------|
| `inuse_space` | 当前**存活**对象占用的**字节** | 默认；找"现在堆里有什么"——抓泄漏首选 |
| `inuse_objects` | 当前**存活**对象的**个数** | 判断是"少数大对象"还是"海量小对象"泄漏 |
| `alloc_space` | **累计**分配过的**字节** | 找"谁分配得最猛"——优化 GC 压力首选 |
| `alloc_objects` | **累计**分配过的对象**个数** | 找"谁分配次数最多"——小对象优化 |

**`inuse` vs `alloc` 是核心区别：**

- `inuse_*` 只看**当前还活着的**（没被 GC 回收的）。涨说明回收不掉 → 泄漏信号。
- `alloc_*` 看**历史累计分配**（包括早就被 GC 掉的）。高说明分配压力大 → GC 压力信号，**不是泄漏信号**。

**💡 D2 锚点 — 新手最大的坑：用 `alloc_space` 找泄漏。** 累计分配永远只增不减（除非重启），
看 `alloc` 涨判断泄漏是错的。判断泄漏用 `inuse_space`。判断"分配太多拖慢 GC"才用 `alloc_*`。

切换示例：

```bash
go tool pprof -top -inuse_space    heap.prof   # 默认
go tool pprof -top -inuse_objects  heap.prof
go tool pprof -top -alloc_space    heap.prof
go tool pprof -top -alloc_objects  heap.prof
```

四个都跑一遍对比，能精准判断问题的"形状"（见 [七、heap profile](#七heap-profile原理四种维度泄漏判断)）。

### 3.4 CPU profile 不需要切换 sample 类型

CPU profile 只有"CPU 时间"一个维度。你看到的 `flat`/`cum` 单位都是秒（或采样数）。
heap 才有四种切换，因为内存问题有两种形态（泄漏 vs 分配压力），需要不同维度区分。

---

## 四、工具链：`go tool pprof` 用法全解

`go tool pprof` 是 Go 工具链自带的命令，所有分析都在它里做。三种用法：

### 4.1 三种调用方式

```bash
# 1. 从 HTTP server 实时拉 + 进入交互 shell（最常用）
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30

# 2. 加载本地 .prof 文件 + 进入交互 shell
go tool pprof cpu.prof

# 3. 直接出报告（不进 shell），适合脚本
go tool pprof -top -cum cpu.prof
go tool pprof -text -inuse_space heap.prof
```

第 1 种会在拉取期间阻塞（CPU profile 要采 30 秒）；第 2、3 种是离线分析，秒开。

### 4.2 交互式命令

进 shell 后常用命令：

| 命令 | 作用 |
|------|------|
| `top` | 默认按 flat 排序，列出 top 10 |
| `top -cum` | 按 cum 排序，列出 top 10（**找调用链首选**） |
| `top20` / `top50` | 列更多 |
| `list <函数名>` | 显示该函数源码 + 每一行消耗的值（**最实用**） |
| `list <正则>` | 函数名匹配 |
| `web` | 在浏览器画调用图（需安装 graphviz） |
| `svg` | 把调用图存成 SVG 文件 |
| `png` / `gif` | 调用图存成图片（需 graphviz） |
| `traces` | 打印所有样本的完整栈（debug 用） |
| `peek <正则>` | 看匹配节点的上下游调用关系 |
| `help` | 所有命令 |

`top` 和 `list` 两个命令能解决 80% 的问题。`web` 是图形化的调用图（不是火焰图），
箭头粗细代表流量大小。

### 4.3 Web UI + 火焰图

最直观的分析方式是开 Web UI：

```bash
go tool pprof -http=:8080 cpu.prof
# 浏览器打开 http://localhost:8080
```

里面有几个 tab：

- **VIEW → FLAME GRAPH**：火焰图，最直观。横轴是占比，纵轴是调用栈，最宽的"平顶"就是热点。
- **VIEW → PEEK**：调用关系树
- **VIEW → TOP**：等价于命令行的 `top`
- **VIEW → SOURCE**：等价于 `list`
- **SAMPLE** 菜单（heap 专属）：切换 `inuse_space` / `inuse_objects` / `alloc_space` / `alloc_objects`
- **REFINE → Focus**：只看匹配某个正则的子图

**💡 D2 锚点 — 火焰图怎么读：**

- 横轴宽度 = CPU 时间（或字节、对象数）占比，**不是时间顺序**。
- 纵轴是调用栈：下面的函数调用上面的函数（`main` 在最底）。
- 找**又宽又平的顶**——一个色块横跨很宽，说明大量时间花在这一个函数上，没有再往下细分。
  这通常是热点本身。
- 一个"瘦高"的塔状结构（每层都很窄）是正常的深调用链，不是问题。
- 两个相邻的宽块代表"两件差不多耗资源的事"，可能要分别看。

**看火焰图的前提**：本机要装 `graphviz`（`brew install graphviz` / `apt install graphviz`），
否则 `web` 命令和 Web UI 的图都画不出来。

### 4.4 diff：用 `-base` 对比两份 profile

排查内存泄漏、验证修复效果的核心命令：

```bash
# after 相对 before 的"变化"
go tool pprof -base before.prof after.prof
```

进入 shell 后 `top`，看到的数值是 **after 减 before 的增量**——正数是新增加的，负数是减少的。

适用场景：

- **内存泄漏**：先抓 baseline（空载），再抓 leaking（堆高位），diff 只看增长部分。
- **修复验证**：抓 before（有 bug）和 after（已修复），diff 确认热点消失。
- **优化对比**：抓优化前、优化后两份 CPU profile，diff 看哪些函数 CPU 降了。

**💡 D2 锚点 — diff 是排查的灵魂：** 单看一个 profile 永远分不清"正常该有的"和"问题"。
对比才是 pprof 的方法论——这是为什么 [troubleshooting/](troubleshooting/) 三个 lab 全程强调
`baseline / before / after` 三件套。

### 4.5 常用 flag 速查

| flag | 作用 |
|------|------|
| `-seconds=N` | CPU profile 采样 N 秒（默认 30） |
| `-http=:PORT` | 开 Web UI |
| `-top` | 直接打印 top，不进 shell |
| `-text` | 等价 `-top` |
| `-inuse_space` / `-inuse_objects` / `-alloc_space` / `-alloc_objects` | heap 的 sample 切换 |
| `-base=<file>` | diff 模式 |
| `-nodecount=N` | 图里最多显示多少节点 |
| `-nodefraction=0.x` | 隐藏占比低于 x 的节点（默认 0.005，降噪） |

---

## 五、在 go-zero 服务里启用 pprof

本项目用 [go-zero](https://github.com/zeromicro/go-zero) 框架，pprof **已经内置**，不用手动 import。

### 5.1 go-zero 的 DevServer

go-zero 的 `rest.Server`（HTTP）和 `zrpc.RpcServer`（gRPC）都内置一个 `DevServer`，
跑在业务端口之外的独立端口上，默认监听 `localhost`。pprof 就挂在 DevServer 上。

本项目的端口分配（来自 [troubleshooting/README.md](troubleshooting/README.md)）：

| 服务 | pprof 端口 |
|------|-----------|
| order-api | 6060 |
| user-api | 6061 |
| user-rpc | 6062 |
| inventory-rpc | 6063 |

启动任意服务后，直接 curl 验证：

```bash
curl http://localhost:6060/debug/pprof/
```

返回 HTML 列出所有 endpoint（`profile`、`heap`、`goroutine` 等），就说明 OK。

### 5.2 在非 go-zero 程序里手动接

如果是自己写的 `net/http` 服务，最小写法：

```go
import _ "net/http/pprof"

go func() {
    log.Println(http.ListenAndServe("127.0.0.1:6060", nil))
}()
```

注意：

- `http.DefaultServeMux` 是被 `net/http/pprof` 注册的目标，所以 `ListenAndServe` 的第二参数必须是 `nil`（用 DefaultServeMux）。
- 如果业务 server 已经在用 `http.DefaultServeMux`，可以单独起一个 server 监听别的端口，避免互相干扰。

---

## 六、CPU profile：原理、采集、分析

### 6.1 原理：100Hz 信号采样

Go runtime 启动一个定时器（基于 `setitimer`，Linux 默认 100Hz），每隔 10ms 给当前线程
发 `SIGPROF` 信号。信号处理器在所有"正在 CPU 上跑"的 goroutine 里抓一个栈，记下来。

跑 30 秒 ≈ 3000 个样本。每个样本是一个栈 + 一个计数。

**💡 D2 锚点 — 采样意味着"统计近似"，不是精确测量：** 30 秒里某个函数被采样到 100 次，
不等于它精确消耗了 10 秒 CPU——只是统计上占 1/30。但只要样本量够（> 几百），相对占比是
可信的。这也是为什么 pprof 给的都是"占比"而不是绝对耗时。

### 6.2 CPU profile 抓不到的东西

以下这些**不会**出现在 CPU profile 里（因为采样时它们没在 CPU 上跑）：

- 卡在 IO 等待的 goroutine（`read(socket)` 等数据）
- 卡在锁、channel 的 goroutine
- 卡在系统调用阻塞里的 goroutine（部分）
- 睡眠中的 goroutine（`time.Sleep`）

它们要去 **goroutine profile** 找，状态会是 `[IO wait]` / `[semacquire]` / `[chan receive]` 等。

### 6.3 怎么采集

```bash
# 方式一：交互式（推荐，最常用）
go tool pprof -seconds=30 http://localhost:6060/debug/pprof/profile

# 方式二：存盘后离线分析
curl -o cpu.prof "http://localhost:6060/debug/pprof/profile?seconds=30"
go tool pprof -http=:8080 cpu.prof
```

**💡 D2 锚点 — 为什么默认 30 秒：** 太短（<5s）样本太少噪声大；太长（>2min）期间代码可能
已变化（比如发版了），定位失真。30s 既能覆盖到足够请求，又不至于太久。

### 6.4 怎么分析（四步法）

1. **`top -cum`** 找调用链上 cum 最高的——这是资源流向。
2. **`top`**（默认按 flat）找 flat 最高的——这是热点本身。
3. **`list <热点函数>`** 进函数看具体哪一行消耗最大。
4. **火焰图**（`-http`）一眼确认，给同事/上级复盘时用。

### 6.5 怎么判断"是业务热点还是 GC / 锁"

看 `top` 里 `runtime.*` 函数的占比：

| 看到的函数 | 含义 | 下一步 |
|------------|------|--------|
| 业务函数（你的代码）flat 高 | 业务热点 | 改业务代码 |
| `runtime.gcBgMarkWorker` / `runtime.mallocgc` 高 | GC 压力（分配过多） | 查 heap 的 `alloc_space`，减少分配 |
| `runtime.asyncPreempt` 高 | 频繁抢占，通常锁竞争或 goroutine 过多 | 查 mutex / block profile |
| `runtime.futex` / `lock` 高 | 锁竞争 | 查 mutex profile |
| `syscall.*` 高 | 系统调用过多（IO、文件） | 看是不是有未 buffer 的 IO 循环 |

**💡 D2 锚点 — GC 高 ≠ 内存泄漏：** `runtime.gcBgMarkWorker` 占比高说明 GC 在拼命干活，
根因通常是**分配太快**（GC 压力），不是泄漏——泄漏的对象 GC 根本碰不到（还被引用着），
反而让 GC 看起来"轻松"。判断泄漏看 heap 的 `inuse_space` 单调性。

### 6.6 实战：去做 lab

读完原理，去 [troubleshooting/cpu-hotspot.md](troubleshooting/cpu-hotspot.md) 动手走一遍完整的
"注入 CPU bug → 抓 profile → top/list/web 定位 → 修复 → diff 验证"流程。那份文档有真实
profile 输出和火焰图解读。

---

## 七、heap profile：原理、四种维度、泄漏判断

### 7.1 原理：基于分配的采样 + 瞬时快照

Go runtime 给每次堆分配计数，每分配 `runtime.MemProfileRate`（默认 512KB）字节
记录一次样本，记下当时的栈和分配大小。`/debug/pprof/heap` 访问的瞬间，runtime 把当前
累计的所有样本 dump 出来。

每个样本同时记录了"分配的字节"和"已经被 GC 回收的字节"，所以同一份数据能算出
`inuse`（存活）和 `alloc`（累计）两种维度。

**💡 D2 锚点 — 采样率 512KB 意味着什么：** 不是每次分配都记，是"每分配 512KB 记一次"。
小对象（几十字节）要分配很多次才命中一次，但累积下来统计上正确。这个采样率是性能和精度
的平衡——开成 1（每次都记）会让程序慢到不能用。

### 7.2 怎么采集

```bash
# 默认抓当前 inuse_space
curl -o heap.prof http://localhost:6060/debug/pprof/heap

# 想确认"真的回收不掉"，抓之前强制 GC 一次
curl -o heap.prof "http://localhost:6060/debug/pprof/heap?gc=1"
```

`?gc=1` 让 pprof 抓之前调一次 `runtime.GC()`，GC 后还在的字节就是被引用着回收不掉的。

### 7.3 四种维度，对比看

同一份 `heap.prof` 切换 sample：

```bash
go tool pprof -top -inuse_space    heap.prof   # 当前存活字节（默认）
go tool pprof -top -inuse_objects  heap.prof   # 当前存活对象数
go tool pprof -top -alloc_space    heap.prof   # 累计分配字节
go tool pprof -top -alloc_objects  heap.prof   # 累计分配对象数
```

| 场景 | 判断 |
|------|------|
| `inuse_space` 持续涨，不回落 | 内存泄漏（字节维度） |
| `inuse_objects` 涨但 `inuse_space` 不突出 | 海量小对象泄漏（map/slice entry 没清） |
| `inuse_space` 涨，`inuse_objects` 不突出 | 少数大对象泄漏（缓存了 blob / payload） |
| `alloc_space` 高但 `inuse_space` 正常 | 分配压力大（GC 压力），**不是泄漏** |
| `alloc_objects` 高但 `alloc_space` 不突出 | 海量小对象分配（频繁创建临时小对象） |

**💡 D2 锚点 — 四种维度组合判断泄漏"形状"：** 这套对比能直接告诉你去 grep 什么样的代码。
大对象泄漏找"缓存/持有 payload 的地方"；小对象泄漏找"循环里 append / map 赋值但没清理的地方"。
GC 压力找"热点函数里反复创建临时对象的地方"。

### 7.4 怎么分析（内存泄漏五步法）

1. **抓 baseline**（空载或刚启动时）：`curl -o baseline.prof .../heap`
2. **触发故障/打流量**，等堆涨起来。
3. **在堆高位抓 leaking**：`curl -o leaking.prof .../heap`
4. **diff**：`go tool pprof -base baseline.prof leaking.prof`，`top` 看增量。
5. **`list <增量最大的函数>`** 进函数看具体分配点。

diff 是关键——单看 leaking 分不清"正常该有的"和"泄漏的"，diff 后只看增长部分，那才是泄漏对象。

### 7.5 内存泄漏 vs GC 压力：最容易混的两个问题

| | 内存泄漏 | GC 压力 |
|--|----------|---------|
| `inuse_space` 趋势 | 单调上涨，不回落 | 波动，GC 后能回落 |
| `alloc_space` | 正常 | 极高（疯狂分配） |
| CPU profile 里 GC 函数 | 占比正常 | `runtime.gcBgMarkWorker`/`mallocgc` 高 |
| 修复方向 | 找泄漏引用，断开它 | 减少分配（sync.Pool、复用对象、减少临时对象） |

**💡 D2 锚点 — 别看到"内存涨"就以为是泄漏：** 先看 `inuse_space` 趋势——单调上涨才是泄漏。
波动是正常的（GC 在干活）。再看 CPU profile 里 GC 函数高不高——高就是 GC 压力，不是泄漏。
两个问题修法完全不同，搞混了白忙活。

### 7.6 实战：去做 lab

去 [troubleshooting/memory-leak.md](troubleshooting/memory-leak.md) 动手走一遍完整的
"注入无淘汰缓存 → baseline → 填充 → diff → 四种维度对比 → 修复"流程。那份文档有真实的
四种 sample 对比表和泄漏形状判断。

---

## 八、goroutine profile：原理、debug 模式、卡点读法

### 8.1 原理：瞬时全量快照

访问 `/debug/pprof/goroutine` 时，runtime 调用 `runtime.Stack(allGoroutines)`，
把**当前所有** goroutine 的栈一次性 dump 出来。没有采样，是真·全量快照。

**💡 D2 锚点 — 和 CPU/heap 的根本不同：** CPU 是基于时间的采样，heap 是基于分配的采样，
goroutine 是**全量**快照——每个 goroutine 都被记录，一个不漏。所以 goroutine profile
最适合"找卡住的具体那一批 goroutine"。

### 8.2 怎么采集

```bash
# 文本格式（推荐排查用 debug=2）
curl -o goroutine.txt "http://localhost:6060/debug/pprof/goroutine?debug=2"

# protobuf 格式（用 go tool pprof 分析）
curl -o goroutine.prof http://localhost:6060/debug/pprof/goroutine
```

两种格式各有所长：

| 格式 | 用途 |
|------|------|
| `debug=1` | 每个栈只出现一次，前面带计数。快速扫"哪类栈最多"。 |
| `debug=2` | **每个 goroutine 单独列**，带完整栈 + 状态 + 阻塞原因 + 卡了多久 + 谁创建的。排查泄漏首选。 |
| 默认（protobuf） | 用 `go tool pprof` 分析，能排序、火焰图。但看不到"卡了多久""谁创建的"这种细节。 |

### 8.3 `debug=2` 输出长什么样

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

一个 goroutine 一段，信息很丰富：

- `goroutine 12345`：goroutine ID（仅用于排查，业务代码不应依赖）
- `[chan receive, 5 minutes]`：**状态** + **卡了多久**——最关键的两个信息
- 中间：完整调用栈，最后一行是你代码里卡住的位置
- `created by ... in goroutine 12340`：**谁 fork 的这个 goroutine**，泄漏排查时顺藤摸瓜用

### 8.4 卡点状态速查

`[...]` 里的状态直接告诉你卡在哪类操作：

| 状态 | 含义 | 常见根因 |
|------|------|----------|
| `[chan receive]` | `<-ch` 接收 | 发送方跑了/没人发，或 channel 被 GC 但接收方还阻塞 |
| `[chan send]` | `ch <- x` 发送 | 无缓冲 channel 接收方没了，或缓冲满且无人收 |
| `[select]` | select 无 case 就绪 | select 漏了 `case <-ctx.Done()` |
| `[semacquire]` / `[sync.Mutex.Lock]` | 锁 | 锁竞争或持锁者没释放 |
| `[IO wait]` / `[netpoll]` | 网络读写 | HTTP client 无超时，对端 hang 住 |
| `[finalizer wait]` | finalizer 队列 | 少见，finalizer 堆积 |
| `[GC sweep wait]` / `[GC scavenge wait]` | GC 内部 | 正常后台活动，一般可忽略 |
| `[sleep]` | `time.Sleep` | 正常，除非卡特别久 |

### 8.5 怎么分析（goroutine 泄漏四步法）

1. **抓两次 profile**，间隔 1~2 分钟。
2. **数同一条栈出现多少次**——出现最多的那条栈就是泄漏源。
3. **diff** 两次，第二次比第一次新增的栈就是泄漏的（正常 goroutine 会在间隔内退出）。
4. **看卡点状态和 `created by`**，定位到具体代码行。

数栈的命令：

```bash
grep -A1 "^goroutine [0-9]" goroutine.txt | \
  grep -v "^--$" | sort | uniq -c | sort -rn | head
```

### 8.6 goroutine 泄漏 vs 运行时死锁

| | Goroutine 泄漏 | 运行时死锁 |
|--|----------------|-----------|
| 本质 | **部分** goroutine 卡住不退出 | **所有** goroutine 全部阻塞 |
| 进程状态 | 活着，还能处理请求（只是 goroutine 数越来越多） | **直接崩溃**，`fatal error: all goroutines are asleep - deadlock!` |
| 抓 profile | 能抓（进程活着） | 抓不到（进程已死），靠崩溃日志里的堆栈 |
| 性质 | 慢病 | 猝死 |

**💡 D2 锚点 — runtime 死锁是 runtime 自己检测的：** Go runtime 的调度器每次循环结束时
检查"还有没有可运行的 goroutine"。如果**所有** goroutine 都阻塞、且没有任何外部事件能让它们
醒来（无网络 IO、无 timer、无系统调用返回），runtime 判定整死，直接 panic。
所以这个 panic 抓不到 profile——进程当场崩。

### 8.7 实战：去看速查 + 动手

goroutine 排查手法套路化，[troubleshooting/goroutine-deadlock-quickref.md](troubleshooting/goroutine-deadlock-quickref.md)
完整覆盖了泄漏和死锁的排查路径、卡点状态表、修复套路。重点读那份的"卡点关键字速判"和
"最经典的三种泄漏"。

---

## 九、生产环境的注意事项

### 9.1 pprof 端口绝对不要暴露公网

pprof endpoint 暴露的是：

- 完整调用栈（你的代码结构、依赖、甚至密钥常量都可能出现在栈里）
- 当前堆内存布局（理论上能 dump对象内容）
- goroutine 列表（可能含请求参数）

绑 `127.0.0.1`，或在反向代理 / 安全组里挡掉 `/debug/pprof/`。

### 9.2 抓 CPU profile 的开销

CPU profile 默认 100Hz 采样，开销很低（约 1~3% CPU）。可以安全在线上跑 30 秒。

注意：采样率可以调（`runtime.SetCPUProfileRate`），但**不要**随意调高——10 倍采样率
不是 10 倍精度，而是 10 倍开销，还可能因为信号风暴影响业务延迟。

### 9.3 抓 heap 的 STW

`/debug/pprof/heap?gc=1` 会触发一次 `runtime.GC()`，GC 会短时间 STW（Stop-The-World）。
线上抓时如果对延迟敏感，避开高峰期。

不带 `gc=1` 抓 heap 不会触发 STW，是读 runtime 已经累计的采样数据。

### 9.4 block / mutex profile 要显式开

这两个默认不采，需要代码里设置：

```go
// block profile：每纳秒记录一次（值越小越精细，开销越大）
runtime.SetBlockProfileRate(1)        // 1ns 精度，开销大，定位时短开
runtime.SetBlockProfileRate(1_000_000) // 1ms 精度，开销小

// mutex profile：记录比例（1=全采，10=采 1/10）
runtime.SetMutexProfileFraction(1)
```

定位完记得关掉（设 0），否则长期开会拖慢服务。

---

## 十、命令速查总表

### 采集

```bash
# CPU profile（30 秒，交互式）
go tool pprof -seconds=30 http://localhost:6060/debug/pprof/profile

# CPU profile（存盘）
curl -o cpu.prof "http://localhost:6060/debug/pprof/profile?seconds=30"

# heap profile（默认 inuse_space）
curl -o heap.prof http://localhost:6060/debug/pprof/heap

# heap profile（抓之前强制 GC）
curl -o heap.prof "http://localhost:6060/debug/pprof/heap?gc=1"

# goroutine profile（文本，排查用）
curl -o goroutine.txt "http://localhost:6060/debug/pprof/goroutine?debug=2"

# goroutine profile（protobuf，用 pprof 分析）
curl -o goroutine.prof http://localhost:6060/debug/pprof/goroutine
```

### 分析

```bash
# 进交互 shell
go tool pprof cpu.prof

# 开 Web UI + 火焰图
go tool pprof -http=:8080 cpu.prof

# 直接 top，不进 shell
go tool pprof -top -cum cpu.prof

# heap 切 sample 维度
go tool pprof -top -inuse_space    heap.prof
go tool pprof -top -inuse_objects  heap.prof
go tool pprof -top -alloc_space    heap.prof
go tool pprof -top -alloc_objects  heap.prof

# diff（看增量）
go tool pprof -base baseline.prof leaking.prof
```

### 交互 shell 内

| 命令 | 作用 |
|------|------|
| `top` | 按 flat 排序 top 10 |
| `top -cum` | 按 cum 排序（找调用链首选） |
| `list <函数名>` | 看函数源码 + 每行消耗 |
| `web` | 浏览器画调用图（需 graphviz） |
| `traces` | 打印所有样本完整栈 |
| `help` | 所有命令 |

### 前置依赖

```bash
# graphviz：web / 火焰图 / svg 必需
brew install graphviz     # macOS
apt install graphviz      # Debian/Ubuntu
```

---

## 十一、学习路径建议

按这个顺序走，一周能从零到上手线上排查：

**第 1 天：把概念吃透**

- 读本文档 [一](#一pprof-是什么) 到 [五](#五在-go-zero-服务里启用-pprof)。
- 重点：采样 vs 快照、flat vs cum、heap 的四种 sample。这三个概念不懂，后面看啥都糊。

**第 2-3 天：CPU 主题**

- 读本文档 [六、CPU profile](#六cpu-profile原理采集分析)。
- 做 [troubleshooting/cpu-hotspot.md](troubleshooting/cpu-hotspot.md) 的 lab（启动 BUG_CPU=1 服务 → 抓 profile → top/list/web → 修复）。
- 自己写一个 benchmark，用 `go test -bench -cpuprofile` 抓 profile，体验 benchmark 和线上 profile 的差异。

**第 4-5 天：内存主题**

- 读本文档 [七、heap profile](#七heap-profile原理四种维度泄漏判断)。
- 做 [troubleshooting/memory-leak.md](troubleshooting/memory-leak.md) 的 lab。
- 重点练：四种 sample 维度的对比、`-base` diff、判断"泄漏 vs GC 压力"。

**第 6-7 天：goroutine 主题**

- 读本文档 [八、goroutine profile](#八goroutine-profile原理debug-模式卡点读法)。
- 读 [troubleshooting/goroutine-deadlock-quickref.md](troubleshooting/goroutine-deadlock-quickref.md)（这份是速查，没 lab）。
- 自己写个小程序制造 goroutine 泄漏（`go func() { <-ch }()` 不传 ctx），抓 profile 排查。

**进阶：** block profile、mutex profile、`go tool trace`（事件时间线）、`runtime/trace`
（调度分析）。这些是 pprof 解决不了的问题时才用的，先别一上来就学。

---

## 附：常见误区清单

1. **用 `alloc_space` 找内存泄漏** → 错。累计分配永远只增不减。用 `inuse_space`。
2. **CPU profile 里看不到某函数就以为它不耗时** → 错。它可能在 IO 等待、锁等待，去 goroutine profile 找。
3. **`flat` 和 `cum` 分不清** → flat 是函数自己，cum 是自己 + 子调用。找热点看 flat，追链路看 cum。
4. **抓一次 profile 就下结论** → 错。采样是统计近似，单次可能噪声大。重要结论多抓几次对比。
5. **内存泄漏排查不抓 baseline** → 错。单看一份 heap 分不清"正常的"和"泄漏的"。必须 diff。
6. **goroutine 数高就是泄漏** → 不一定。看趋势：稳定不涨是正常的；单调上涨或阶梯跳变后不回落才是泄漏。
7. **`runtime.gcBgMarkWorker` 高就以为内存泄漏** → 错。GC 高是分配压力（GC 压力），
   判断泄漏看 `inuse_space` 趋势。
8. **生产环境把 pprof 端口暴露公网** → 危险。会泄露代码结构和内存内容。绑内网。
9. **抓 CPU profile 时间太短（<5s）** → 样本太少，噪声大。至少 30s。
10. **不装 graphviz 就用 `web`** → 报错。`brew install graphviz` 先。

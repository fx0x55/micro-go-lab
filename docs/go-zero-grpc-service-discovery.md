# 深入 go-zero：从 etcd 到 gRPC，服务注册 / 发现 / 负载均衡的完整链路

> 本文基于 go-zero v1.10.2 + gRPC v1.81.1 源码，逐行追踪服务注册、服务发现、负载均衡的完整调用链。所有代码引用均可在源码中定位。

---

## 一句话总结

go-zero 的服务发现本质上是：**etcd watch 推送地址 → gRPC Resolver 接收 → Balancer 创建连接 → P2C Picker 选后端**。go-zero 负责"发现"和"选择"，gRPC 负责"连接管理"。

---

## 全景架构

```
┌─────────────────────────────────────────────────────────────────────┐
│                         Server 端 (user-rpc)                        │
│                                                                     │
│  zrpc.Server 启动                                                   │
│       │                                                             │
│       ▼                                                             │
│  Publisher.register()                                               │
│       │  etcd.Put("user-svc.rpc/<leaseID>", "10.0.0.1:9090")       │
│       │  绑定 lease (TTL=10s)                                       │
│       ▼                                                             │
│  ┌─────────┐    lease keepalive    ┌──────────┐                     │
│  │  user-rpc │ ──────────────────→ │   etcd   │                     │
│  └─────────┘                       └──────────┘                     │
└─────────────────────────────────────────────────────────────────────┘
                                     │
                          watch 推送 PUT/DELETE 事件
                                     │
┌─────────────────────────────────────────────────────────────────────┐
│                         Client 端 (order-api)                       │
│                                                                     │
│  zrpc.MustNewClient(conf)                                           │
│       │                                                             │
│       ├─ BuildTarget() → "etcd:///localhost:2379?key=user-svc.rpc"  │
│       ├─ makeLBServiceConfig() → {"loadBalancingPolicy":"p2c_ewma"} │
│       │                                                             │
│       ▼                                                             │
│  grpc.Dial(target)                                                  │
│       │  识别 scheme="etcd" → etcdBuilder.Build()                   │
│       │                                                             │
│       ▼                                                             │
│  discov.NewSubscriber() → Registry.Monitor()                        │
│       │  cluster.load()  → etcd GET (初始地址)                       │
│       │  cluster.watch() → etcd WATCH (实时变更)                     │
│       │                                                             │
│       ▼                                                             │
│  container.notifyChange() → update() → cc.UpdateState(Addresses)    │
│       │                                                             │
│       ▼                                                             │
│  gRPC baseBalancer: 为每个地址创建 SubConn                           │
│       │                                                             │
│       ▼                                                             │
│  p2cPicker.Pick(): 每次 RPC 随机选 2 个 → 比较 load → 选优          │
└─────────────────────────────────────────────────────────────────────┘
```

---

## 一、服务注册：Server 端如何把自己"挂到" etcd 上

### 1.1 触发时机

zrpc.Server 启动时，内部创建 `Publisher`，调用 `register()` 将自身地址写入 etcd。

### 1.2 源码解析

**文件：`core/discov/publisher.go:185-200`**

```go
func (p *Publisher) register(client internal.EtcdClient) (clientv3.LeaseID, error) {
    // 1. 申请一个带 TTL 的 lease（默认 10 秒）
    resp, err := client.Grant(client.Ctx(), TimeToLive)
    if err != nil {
        return clientv3.NoLease, err
    }

    lease := resp.ID
    // 2. 构造 key：前缀 + leaseID，例如 "user-svc.rpc/7587895049307938642"
    if p.id > 0 {
        p.fullKey = makeEtcdKey(p.key, p.id)
    } else {
        p.fullKey = makeEtcdKey(p.key, int64(lease))
    }
    // 3. 写入 etcd，绑定 lease
    _, err = client.Put(client.Ctx(), p.fullKey, p.value, clientv3.WithLease(lease))
    return lease, err
}
```

### 1.3 关键设计

| 维度 | 实现 |
|------|------|
| **key 格式** | `<服务前缀>/<leaseID>`，如 `user-svc.rpc/7587895049307938642` |
| **value** | 服务端地址，如 `10.0.0.1:9090` |
| **健康保障** | etcd lease 自动续租；进程崩溃 → lease 过期 → key 自动删除 |
| **多实例** | 每个实例拿到不同 leaseID，key 不冲突 |

这意味着：**服务注册是"声明式"的，不需要心跳接口。** etcd 的 lease 机制天然解决了"服务挂了但没通知"的问题。

---

## 二、服务发现：Client 端如何找到所有可用实例

### 2.1 入口：用户代码只需两行

```go
conf := zrpc.NewEtcdClientConf([]string{"localhost:2379"}, "user-svc.rpc", "", "")
cli := zrpc.MustNewClient(conf)
```

### 2.2 配置构造

**文件：`zrpc/config.go:66-75`**

```go
func NewEtcdClientConf(hosts []string, key, app, token string) RpcClientConf {
    return RpcClientConf{
        Etcd: discov.EtcdConf{
            Hosts: hosts,  // ["localhost:2379"]
            Key:   key,    // "user-svc.rpc"
        },
    }
}
```

此时只是数据结构，没有任何网络操作。

### 2.3 构造 gRPC Target URL

**文件：`zrpc/config.go:92-114`** → `zrpc/resolver/target.go:18-23`

```go
// BuildTarget() 内部调用:
func BuildDiscovTarget(endpoints []string, key string) string {
    return fmt.Sprintf("%s:///%s?key=%s", internal.EtcdScheme,
        strings.Join(endpoints, internal.EndpointSep), url.QueryEscape(key))
}
```

拼出的 target：

```
etcd:///localhost:2379?key=user-svc.rpc
```

这是一个标准的 gRPC URI：scheme 是 `etcd`，host 在 path 中，key 作为 query 参数。

### 2.4 Resolver 注册：gRPC 如何认识 `etcd` 这个 scheme

**文件：`zrpc/internal/client.go:22-24`**

```go
func init() {
    resolver.Register()  // 包加载时自动执行
}
```

**文件：`zrpc/resolver/internal/resolver.go:33-37`**

```go
func register() {
    resolver.Register(&directResolverBuilder)   // scheme "direct"
    resolver.Register(&discovResolverBuilder)   // scheme "discov"
    resolver.Register(&etcdResolverBuilder)     // scheme "etcd"
}
```

go-zero 在 gRPC 的全局 resolver 注册表中挂了三个 builder。当 gRPC dial 一个 `etcd:///...` 的 target 时，会找到 `etcdBuilder`。

### 2.5 Resolver Build：连接 etcd 并启动 watch

**文件：`zrpc/resolver/internal/discovbuilder.go:14-45`**

```go
func (b *discovBuilder) Build(target resolver.Target, cc resolver.ClientConn, ...) (
    resolver.Resolver, error) {

    // 1. 从 target URL 解析出 etcd hosts 和 key
    hosts := strings.FieldsFunc(targets.GetHosts(target), ...)  // ["localhost:2379"]
    sub, err := discov.NewSubscriber(hosts, targets.GetKey(target))  // "user-svc.rpc"

    // 2. 定义 update 回调：每次实例变化时推送给 gRPC
    update := func() {
        vals := subset(sub.Values(), subsetSize)  // 去重，随机打散，最多 32 个
        addrs := make([]resolver.Address, 0, len(vals))
        for _, val := range vals {
            addrs = append(addrs, resolver.Address{Addr: val})
        }
        cc.UpdateState(resolver.State{Addresses: addrs})  // ← 喂给 gRPC
    }
    sub.AddListener(update)  // 注册回调
    update()                 // 立即执行一次，获取初始地址列表

    return &discovResolver{cc: cc, sub: sub}, nil
}
```

**这是整个服务发现的枢纽。** `cc.UpdateState()` 是 gRPC 定义的接口（`grpc/resolver/resolver.go:243`），go-zero 通过它把发现的地址推送给 gRPC。

### 2.6 Subscriber：从 Registry 到 etcd

**文件：`core/discov/subscriber.go:31-48`**

```go
func NewSubscriber(endpoints []string, key string, opts ...SubOption) (*Subscriber, error) {
    sub := &Subscriber{endpoints: endpoints, key: key}
    sub.items = newContainer(sub.exclusive)  // KV 存储容器

    // 桥接到 Registry 层
    internal.GetRegistry().Monitor(endpoints, key, sub.exactMatch, sub.items)
    return sub, nil
}
```

### 2.7 Registry.Monitor：连接复用 + watch 共享

**文件：`core/discov/internal/registry.go:51-78`**

```go
func (r *Registry) Monitor(endpoints []string, key string, exactMatch bool, l UpdateListener) error {
    wkey := watchKey{key: key, exactMatch: exactMatch}

    c, exists := r.getOrCreateCluster(endpoints)
    if exists {
        // 同一个 etcd 集群已有连接 → 复用
        c.lock.Lock()
        watcher, ok := c.watchers[wkey]
        if ok {
            watcher.listeners = append(watcher.listeners, l)
        }
        c.lock.Unlock()

        if ok {
            // 同一个 key 已有 watch → 新 listener 直接回放当前值
            kvs := c.getCurrent(wkey)
            for _, kv := range kvs {
                l.OnAdd(kv)
            }
            return nil
        }
    }

    return c.monitor(wkey, l)  // 首次：建连 + load + watch
}
```

两个关键优化：
- **集群共享**：多个 subscriber 连同一个 etcd 集群时，复用同一个 TCP 连接
- **watch 共享**：多个 subscriber watch 同一个 key 时，复用同一个 etcd watch channel；新 listener 通过 `OnAdd` 回放当前值

### 2.8 cluster.monitor：初始加载 + watch 流

**文件：`core/discov/internal/registry.go:334-347`**

```go
func (c *cluster) monitor(key watchKey, l UpdateListener) error {
    cli, err := c.getClient()       // 获取 etcd 客户端
    c.addListener(key, l)
    rev := c.load(cli, key)         // ← 第一步：GET 加载所有已注册实例
    c.watchGroup.Run(func() {
        c.watch(cli, key, rev)      // ← 第二步：WATCH 监听后续变更
    })
    return nil
}
```

### 2.9 cluster.load：初始全量加载

**文件：`core/discov/internal/registry.go:301-332`**

```go
func (c *cluster) load(cli EtcdClient, key watchKey) int64 {
    var resp *clientv3.GetResponse
    for {
        ctx, cancel := context.WithTimeout(cli.Ctx(), RequestTimeout)
        // 前缀匹配：etcd GET "user-svc.rpc/" WITH PREFIX
        resp, err = cli.Get(ctx, makeKeyPrefix(key.key), clientv3.WithPrefix())
        cancel()
        if err == nil {
            break
        }
        // 失败重试，带退避
        time.Sleep(coolDownUnstable.AroundDuration(coolDownInterval))
    }

    kvs := make([]KV, 0, len(resp.Kvs))
    for _, ev := range resp.Kvs {
        kvs = append(kvs, KV{Key: string(ev.Key), Val: string(ev.Value)})
    }
    c.handleChanges(key, kvs)
    return resp.Header.Revision  // 返回 etcd revision，供 watch 断点续传
}
```

`makeKeyPrefix` 会给 key 加 `/` 后缀：`user-svc.rpc` → `user-svc.rpc/`，这样 etcd 的前缀查询能匹配到所有子 key（如 `user-svc.rpc/7587895049307938642`）。

### 2.10 cluster.watch：实时事件流

**文件：`core/discov/internal/registry.go:387-403`**

```go
func (c *cluster) watch(cli EtcdClient, key watchKey, rev int64) {
    for {
        err := c.watchStream(cli, key, rev)
        if err == nil {
            return
        }
        // watch 被压缩（etcd 清理了旧 revision）→ 重新 load
        if rev != 0 && errors.Is(err, rpctypes.ErrCompacted) {
            rev = c.load(cli, key)
        }
        // 失败重试，带退避（防止 CPU/磁盘打满）
        time.Sleep(coolDownUnstable.AroundDuration(coolDownInterval))
    }
}
```

`watchStream` 内部调用 `cli.Watch()` 从 `rev+1` 开始监听，保证不漏事件。

### 2.11 handleWatchEvents：etcd 事件 → 业务通知

**文件：`core/discov/internal/registry.go:252-299`**

```go
func (c *cluster) handleWatchEvents(ctx context.Context, key watchKey, events []*clientv3.Event) {
    for _, ev := range events {
        switch ev.Type {
        case clientv3.EventTypePut:
            evKey := string(ev.Kv.Key)
            evVal := string(ev.Kv.Value)
            oldVal, exists := watcher.values[evKey]
            watcher.values[evKey] = evVal
            if exists && oldVal == evVal {
                continue  // 值没变，跳过（防止无界增长）
            }
            if exists {
                // key 的值变了：先通知删除旧值，再通知添加新值
                for _, l := range listeners {
                    l.OnDelete(KV{Key: evKey, Val: oldVal})
                }
            }
            for _, l := range listeners {
                l.OnAdd(KV{Key: evKey, Val: evVal})
            }
        case clientv3.EventTypeDelete:
            delete(watcher.values, string(ev.Kv.Key))
            for _, l := range listeners {
                l.OnDelete(KV{Key: string(ev.Kv.Key), Val: string(ev.Kv.Value)})
            }
        }
    }
}
```

三种 etcd 事件的处理：

| 事件 | 含义 | 处理 |
|------|------|------|
| PUT（新 key） | 新实例上线 | `OnAdd` → container 增加地址 |
| PUT（已有 key） | 实例地址变更 | `OnDelete` 旧值 + `OnAdd` 新值 |
| DELETE | 实例下线（lease 过期） | `OnDelete` → container 移除地址 |

### 2.12 Container：地址的最终归宿

**文件：`core/discov/subscriber.go:109-137`**

```go
type container struct {
    exclusive bool
    values    map[string][]string  // 地址 → [key1, key2, ...]
    mapping   map[string]string    // key → 地址
    snapshot  atomic.Value         // 快照，供 Values() 无锁读取
    listeners []func()             // 变更监听器
}

func (c *container) OnAdd(kv internal.KV) {
    c.addKv(kv.Key, kv.Val)
    c.notifyChange()  // 触发所有 listener
}

func (c *container) OnDelete(kv internal.KV) {
    c.removeKey(kv.Key)
    c.notifyChange()
}
```

`notifyChange()` 调用所有注册的 listener——包括 2.5 中 `discovBuilder.Build()` 注册的 `update()` 回调，从而形成完整闭环：

```
etcd event → Registry → Container → notifyChange() → update() → cc.UpdateState()
```

---

## 三、地址如何变成 gRPC 可用的连接

到这里，go-zero 的工作完成了。`cc.UpdateState()` 把地址列表交给 gRPC，接下来是 gRPC 自己的领域。

### 3.1 gRPC 的两个核心概念

在深入代码前，先区分两个概念：

- **SubConn**：gRPC 内部对一个后端地址的抽象，封装了一个 TCP 连接（或 HTTP/2 连接池）
- **Picker**：每次 RPC 调用时，从所有可用的 SubConn 中选一个的策略

### 3.2 gRPC 接收地址

**文件：`grpc/resolver/resolver.go:232-257`**

```go
type ClientConn interface {
    UpdateState(State) error  // go-zero 调用的就是这个方法
    ReportError(error)
    // ...
}
```

gRPC 的 `ClientConn` 实现了这个接口。收到 `UpdateState` 后，内部调用 `balancerWrapper.updateClientConnState()`。

### 3.3 baseBalancer：为每个地址创建 SubConn

**文件：`grpc/balancer/base/balancer.go:95-145`**

```go
func (b *baseBalancer) UpdateClientConnState(s balancer.ClientConnState) error {
    addrsSet := resolver.NewAddressMapV2[any]()

    // 遍历 resolver 推送的每个地址
    for _, a := range s.ResolverState.Addresses {
        addrsSet.Set(a, nil)
        if _, ok := b.subConns.Get(a); !ok {
            // 新地址 → 创建 SubConn
            sc, err := b.cc.NewSubConn([]resolver.Address{a}, balancer.NewSubConnOptions{
                HealthCheckEnabled: b.config.HealthCheck,
                StateListener:      func(scs balancer.SubConnState) { b.updateSubConnState(sc, scs) },
            })
            b.subConns.Set(a, sc)
            b.scStates[sc] = connectivity.Idle
            sc.Connect()  // 触发 TCP 连接
        }
    }

    // 移除不再存在的地址
    for a, sc := range b.subConns.All() {
        if _, ok := addrsSet.Get(a); !ok {
            sc.Shutdown()
            b.subConns.Delete(a)
        }
    }

    // 收集所有 Ready 的 SubConn，生成新的 Picker
    b.regeneratePicker()
    b.cc.UpdateState(balancer.State{ConnectivityState: b.state, Picker: b.picker})
    return nil
}
```

### 3.4 regeneratePicker：从 Ready SubConn 构建 Picker

**文件：`grpc/balancer/base/balancer.go:165-179`**

```go
func (b *baseBalancer) regeneratePicker() {
    if b.state == connectivity.TransientFailure {
        b.picker = NewErrPicker(b.mergeErrors())
        return
    }
    readySCs := make(map[balancer.SubConn]SubConnInfo)
    for addr, sc := range b.subConns.All() {
        if st, ok := b.scStates[sc]; ok && st == connectivity.Ready {
            readySCs[sc] = SubConnInfo{Address: addr}
        }
    }
    // 调用 go-zero 注册的 p2cPickerBuilder.Build()
    b.picker = b.pickerBuilder.Build(PickerBuildInfo{ReadySCs: readySCs})
}
```

**这就是 gRPC 和 go-zero 的交接点：** gRPC 收集所有 Ready 的 SubConn，通过 `pickerBuilder.Build()` 交给 go-zero 的 p2c 实现。

### 3.5 连接状态机

每个 SubConn 经历的状态转换：

```
Idle → Connecting → Ready
                  → TransientFailure → (重试) → Connecting
                  → Shutdown
```

当 SubConn 状态变化时，`baseBalancer` 重新调用 `regeneratePicker()`，更新 Picker。这意味着：**某个后端挂了，gRPC 会自动从 Picker 中移除它，下一次 RPC 不会选到它。**

---

## 四、负载均衡：P2C EWMA 算法详解

### 4.1 注册

**文件：`zrpc/internal/balancer/p2c/p2c.go:43-45, 71-73`**

```go
func init() {
    balancer.Register(newBuilder())
}

func newBuilder() balancer.Builder {
    return base.NewBalancerBuilder(Name, new(p2cPickerBuilder), base.Config{HealthCheck: true})
}
```

go-zero 在 `init()` 时将 `p2c_ewma` 注册到 gRPC 的全局 balancer 注册表。客户端通过 service config 指定使用它：

```json
{"loadBalancingPolicy": "p2c_ewma"}
```

### 4.2 Picker 构建

**文件：`zrpc/internal/balancer/p2c/p2c.go:49-69`**

```go
func (b *p2cPickerBuilder) Build(info base.PickerBuildInfo) balancer.Picker {
    readySCs := info.ReadySCs
    if len(readySCs) == 0 {
        return base.NewErrPicker(balancer.ErrNoSubConnAvailable)
    }

    conns := make([]*subConn, 0, len(readySCs))
    for conn, connInfo := range readySCs {
        conns = append(conns, &subConn{
            addr:    connInfo.Address,
            conn:    conn,
            success: initSuccess,
        })
    }
    return &p2cPicker{conns: conns, ...}
}
```

每个 Ready SubConn 被包装成 `subConn`，包含延迟、并发数、成功率等负载指标。

### 4.3 Pick：每次 RPC 的选择

**文件：`zrpc/internal/balancer/p2c/p2c.go:82-119`**

```go
func (p *p2cPicker) Pick(_ balancer.PickInfo) (balancer.PickResult, error) {
    p.lock.Lock()
    defer p.lock.Unlock()

    var chosen *subConn
    switch len(p.conns) {
    case 0:
        return emptyPickResult, balancer.ErrNoSubConnAvailable
    case 1:
        chosen = p.choose(p.conns[0], nil)
    case 2:
        chosen = p.choose(p.conns[0], p.conns[1])
    default:
        // 3+ 个后端：随机选 2 个候选，最多尝试 3 次找到健康的
        var node1, node2 *subConn
        for i := 0; i < pickTimes; i++ {
            a := p.r.Intn(len(p.conns))
            b := p.r.Intn(len(p.conns) - 1)
            if b >= a {
                b++
            }
            node1 = p.conns[a]
            node2 = p.conns[b]
            if node1.healthy() && node2.healthy() {
                break
            }
        }
        chosen = p.choose(node1, node2)
    }

    atomic.AddInt64(&chosen.inflight, 1)
    atomic.AddInt64(&chosen.requests, 1)

    return balancer.PickResult{
        SubConn: chosen.conn,
        Done:    p.buildDoneFunc(chosen),
    }, nil
}
```

### 4.4 choose：两个节点怎么比

**文件：`zrpc/internal/balancer/p2c/p2c.go:164-182`**

```go
func (p *p2cPicker) choose(c1, c2 *subConn) *subConn {
    start := int64(timex.Now())
    if c2 == nil {
        atomic.StoreInt64(&c1.pick, start)
        return c1
    }

    // 比较 load，c1 始终是负载更低的那个
    if c1.load() > c2.load() {
        c1, c2 = c2, c1
    }

    // 防止 c2 长期不被选中（饥饿保护）
    pick := atomic.LoadInt64(&c2.pick)
    if start-pick > forcePick && atomic.CompareAndSwapInt64(&c2.pick, pick, start) {
        return c2
    }

    atomic.StoreInt64(&c1.pick, start)
    return c1
}
```

核心逻辑：
1. 随机选 2 个节点，比较 load
2. 选 load 低的那个
3. 但如果另一个节点很久没被选中（>1s），强制选它（防止饥饿）

### 4.5 load 计算公式

**文件：`zrpc/internal/balancer/p2c/p2c.go:217-226`**

```go
func (c *subConn) load() int64 {
    lag := int64(math.Sqrt(float64(atomic.LoadUint64(&c.lag) + 1)))
    load := lag * (atomic.LoadInt64(&c.inflight) + 1)
    if load == 0 {
        return penalty
    }
    return load
}
```

**`load = sqrt(lag) × (inflight + 1)`**

- `lag`：请求延迟的 EWMA（指数加权移动平均），单位纳秒
- `inflight`：当前正在处理的请求数
- `sqrt(lag)`：对延迟做平方根，降低延迟波动的影响
- `+1`：避免乘零

### 4.6 Done 回调：EWMA 更新

**文件：`zrpc/internal/balancer/p2c/p2c.go:121-162`**

```go
func (p *p2cPicker) buildDoneFunc(c *subConn) func(info balancer.DoneInfo) {
    start := int64(timex.Now())
    return func(info balancer.DoneInfo) {
        atomic.AddInt64(&c.inflight, -1)  // 并发数 -1
        now := timex.Now()
        last := atomic.SwapInt64(&c.last, int64(now))
        td := int64(now) - last

        // EWMA 衰减：越新的请求权重越高
        w := math.Exp(float64(-td) / float64(decayTime))  // decayTime = 10s
        lag := int64(now) - start
        olag := atomic.LoadUint64(&c.lag)
        if olag == 0 {
            w = 0  // 首次请求直接用当前值
        }
        // new_lag = old_lag * w + current_lag * (1 - w)
        atomic.StoreUint64(&c.lag, uint64(float64(olag)*w+float64(lag)*(1-w)))

        // 成功率同样用 EWMA 更新
        success := initSuccess
        if info.Err != nil && !codes.Acceptable(info.Err) {
            success = 0
        }
        osucc := atomic.LoadUint64(&c.success)
        atomic.StoreUint64(&c.success, uint64(float64(osucc)*w+float64(success)*(1-w)))
    }
}
```

每次 RPC 结束后，`Done` 回调自动更新该后端的延迟和成功率指标，供下一次 `Pick()` 使用。

### 4.7 P2C vs 其他算法

| 算法 | 原理 | 优点 | 缺点 |
|------|------|------|------|
| **Round Robin** | 轮询 | 简单 | 不感知负载 |
| **Random** | 随机 | 简单 | 不感知负载 |
| **Least Connections** | 选连接数最少的 | 感知并发 | 需要全局状态 |
| **P2C EWMA** | 随机选 2 个，比较 load | 兼顾随机性和负载感知 | 实现复杂 |

P2C 的核心优势：**每次只需要比较 2 个节点，O(1) 复杂度，却能逼近全局最优。** 这是 Netflix 在内部实践中验证过的算法，被 gRPC 官方推荐。

---

## 五、完整调用链：一行代码的 10 次跳转

以 `zrpc.MustNewClient(conf)` 为例，完整调用链如下：

```
用户代码:
  conf := zrpc.NewEtcdClientConf([]string{"localhost:2379"}, "user-svc.rpc", "", "")
  cli := zrpc.MustNewClient(conf)
       │
       │ ① zrpc/config.go:66 — 构造 RpcClientConf
       ▼
  RpcClientConf{Etcd: {Hosts, Key}}
       │
       │ ② zrpc/client.go:79 — 生成 balancer service config
       ▼
  grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy":"p2c_ewma"}`)
       │
       │ ③ zrpc/config.go:113 — 构造 target URL
       ▼
  "etcd:///localhost:2379?key=user-svc.rpc"
       │
       │ ④ grpc.Dial(target) — gRPC 识别 scheme="etcd"
       ▼
  etcdBuilder.Build()  →  discovBuilder.Build()
       │  zrpc/resolver/internal/discovbuilder.go:14
       │
       │ ⑤ 创建 Subscriber
       ▼
  discov.NewSubscriber(hosts, key)
       │  core/discov/subscriber.go:31
       │
       │ ⑥ 连接 etcd
       ▼
  Registry.Monitor() → cluster.monitor()
       │  core/discov/internal/registry.go:51, 334
       │
       ├─⑦ cluster.load(): etcd GET "user-svc.rpc/" WITH PREFIX
       │    → 获取所有已注册实例地址 + revision
       │
       └─⑧ cluster.watch(): etcd WATCH "user-svc.rpc/" FROM rev+1
            → 实时监听 PUT/DELETE 事件
            │
            │ ⑨ 事件触发 update 回调
            ▼
  handleWatchEvents() → container.OnAdd/OnDelete()
       → notifyChange() → update() → cc.UpdateState(Addresses)
       │
       │ ⑩ gRPC 内部
       ▼
  baseBalancer.UpdateClientConnState()
       → 为每个地址创建 SubConn
       → regeneratePicker() → p2cPickerBuilder.Build(ReadySCs)
       │
       ▼
  后续每次 RPC:
  pickerWrapper.pick() → p2cPicker.Pick()
       → 随机选 2 个 → 比较 sqrt(lag) × (inflight+1)
       → 选 load 低的 → Done 回调更新 EWMA
```

---

## 六、生产环境的健壮性设计

### 6.1 连接复用

同一个 etcd 集群的所有 subscriber 共享一个 TCP 连接（`Registry.Monitor` 中的 `getOrCreateCluster`），避免连接爆炸。

### 6.2 Watch 断点续传

`cluster.load()` 返回 etcd 的 `revision`，`cluster.watch()` 从 `rev+1` 开始监听。即使 watch 断开，重连后能从断点恢复，不丢事件。

### 6.3 Watch 压缩处理

如果 etcd 清理了旧 revision（`ErrCompacted`），`cluster.watch()` 会重新执行 `load()` 全量拉取，再从新 revision 开始 watch。

### 6.4 退避重试

`load()` 和 `watch()` 失败时，使用 `coolDownUnstable.AroundDuration(coolDownInterval)` 做退避重试，防止打满 CPU/磁盘。

### 6.5 Subset 随机打散

`discovBuilder.Build()` 中的 `subset()` 函数会将地址列表随机打散并截断到最多 32 个，防止大集群场景下 gRPC 内部创建过多 SubConn。

### 6.6 P2C 饥饿保护

`choose()` 中的 `forcePick` 机制确保：如果某个后端超过 1 秒没被选中，即使它的 load 稍高也会被选中一次。这防止了"快节点永远被选、慢节点永远闲置"的问题。

---

## 七、总结

| 层级 | 职责 | 关键源码 |
|------|------|----------|
| **服务注册** | Server 写入 etcd，lease 保活 | `core/discov/publisher.go:185` |
| **服务发现** | Client watch etcd，获取实时地址列表 | `core/discov/internal/registry.go:301,387` |
| **地址推送** | Resolver 通过 `cc.UpdateState()` 喂给 gRPC | `zrpc/resolver/internal/discovbuilder.go:32` |
| **连接管理** | gRPC baseBalancer 为每个地址创建 SubConn | `grpc/balancer/base/balancer.go:95` |
| **负载均衡** | P2C EWMA Picker 每次选 2 个比较 load | `zrpc/internal/balancer/p2c/p2c.go:82` |

**go-zero 不创建任何 gRPC 连接，gRPC 不做任何负载均衡决策。** 两者通过 gRPC 的 `resolver.ClientConn` 和 `balancer.PickerBuilder` 两个接口解耦——这正是 gRPC 可插拔架构的精髓。

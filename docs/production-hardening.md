# 生产加固路线图

把本仓库的 go-zero 三服务（user-api / user-rpc / order-api）从"能跑的实验项目"推进到生产级的弹性与正确性基线。
目标：**出事不崩 → 出事能查 → 安全合规 → 可部署可扩展**。

> 详细计划原文见会话计划文件；本文档是其固化版本，供后续会话/协作者直接查阅，避免反复占用上下文。

---

## 两处必须先纠正的前提（探查后确认）

1. **熔断器已存在**：`common/client/user.go` 构造 zrpc 客户端时显式 `Breaker: true`，go-zero 的 GoogleBreaker（SRE 算法）默认挂在 order-api→user-rpc；gRPC 客户端 `Timeout: 2000` 也由 `TimeoutInterceptor` 生效。自定义重试拦截器是最内层，熔断 open 会在重试前短路。**不要再新增熔断器或手动客户端超时**。
2. Phase 0 **无编译修复项**（对话期间用户已修好 `userlogic.go` 的 import）。

---

## 阶段总览

| 阶段 | 主题 | 状态 |
|---|---|---|
| 0 | 止血（限流 IP、负缓存 TTL） | ✅ 完成 2026-06-18 |
| 1 | 弹性与正确性（连接池、ctx 贯通、缓存层、幂等） | ✅ 完成 2026-06-18 |
| 2 | 可观测性完善（结构化日志、Loki、告警） | ✅ 完成 2026-06-18 |
| 3 | 安全加固（TLS、密钥、网关） | ✅ 完成 2026-06-18 |
| 4 | 部署与运维（K8s、CI/CD、探针、HPA） | ⬜ 待做 |
| 5 | 数据一致性与规模化（Outbox、MQ、只读副本） | ⬜ 待做 |

---

## ✅ 阶段 0 + 1（已完成）

### D. 连接池生命周期 + DSN SSL
- `common/config/types.go` `DatabaseConfig` 增 `ConnMaxLifetime`(默认 30m)、`ConnMaxIdleTime`(默认 5m)、`SSLMode`(可选，空=disable)。
- `common/xdb/db.go` DSN 用 `cfg.SSLMode`（空回退 disable），补 `SetConnMaxLifetime`/`SetConnMaxIdleTime`。
- 新增 `(*DatabaseConfig).ApplyEnvOverrides()`，三个服务 config 复用。

### C. ctx 贯通到 repository（16 方法 + 18 调用点）
- 规则：方法首参加 `ctx context.Context`，查询链首加 `r.db.WithContext(ctx)`；调用点传 `l.ctx`。
- 用 `WithContext`（非 `Session`），一次过。无接口消费者，改签名不外溢。
- 涉及 4 个 repository 文件 + 4 个 logic 文件。

### 阶段 0. 限流 IP 解析 + 负缓存 TTL
- `common/middleware/ratelimit.go` 抽 `clientIP(r)`：`X-Forwarded-For` 取逗号前第一个并 trim，回退 `net.SplitHostPort` 去端口。
- 注释：生产应信任网关 `X-Real-IP` 并拒伪造 XFF；当前是进程内限流，多副本需 Redis 版。
- 负缓存 TTL 由阶段 A 的 `CacheConfig.NegativeTTL` 承载（默认 30s）。

### A. 新建 `common/xcache` + 重写 ValidateUser
- 新建 `common/xcache/cache.go`：cache-aside + `golang.org/x/sync/singleflight`（防击穿）+ 负缓存标记（防穿透）+ 显式 `Invalidate`，**nil 安全**（rdb 为 nil 退化为直接回源）。
- 单键设计：值为 JSON 载荷 或 字面量 `__negative__`（原 2 次往返→1 次）。
- `common/config/types.go` 增 `CacheConfig{TTL, NegativeTTL}`；user-rpc config/svc 接入 `Cache *xcache.Cache`。
- ValidateUser 用 `xcache.GetOrLoad`，`ErrRecordNotFound → xcache.ErrMiss`（负缓存）。
- **失效契约**（写在 cache.go 顶部）：任何 user 变更（将来加 UpdateProfile/ChangeUsername）必须 `Invalidate("user:validate:<id>")`；TTL 兜底。
- **修正**：未给 user-api 加 Register 失效钩子（新用户 ID 不可能事先有负缓存，钩子是空操作），避免给 user-api 强加无用 Redis。

### B. 下单幂等（Idempotency-Key + Redis SET NX 单键）
- order-api 首次接入 Redis（svc/config/etc）。
- `Create(userID, req, idempotencyKey)`：key 形如 `order:idem:{sha256(userID+":"+key)}`。
  - `SET NX EX 900` 占位：成功=首请求→创建，成功后把 `json(order)` 写回同一键；失败则 `DEL` 放行重试。
  - 占位失败时 `GET`：空值=进行中→409；非空=已完成→返回缓存 body（重放，Stripe 风格）。
- handler 读 `Idempotency-Key` header（不进 body），映射 `ErrIdempotencyConflict → 409`。
- **无 key 或 Redis nil → 行为不变**（幂等可选，兼容现有 curl，本地无 Redis 仍可用）。

### 新增环境变量
`DATABASE_SSLMODE`、`DATABASE_CONN_MAX_LIFETIME`、`DATABASE_CONN_MAX_IDLE_TIME`、`CACHE_TTL`、`CACHE_NEGATIVE_TTL`、order-api 的 `REDIS_HOST`。

### 验证计划（每步 `make test`；全栈 `make docker-up`）
- **D**：`DATABASE_SSLMODE=require` 打本地非 TLS pg → 连接报错；并发压到 MaxOpenConns 上限观察连接数封顶 25。
- **C**：logic 里 `ctx, cancel()` 后调用 repo → 返回 `context.Canceled`；或 `DATABASE_HOST` 指黑洞地址确认请求在超时报错而非挂死。
- **A 击穿**：50 并发 ValidateUser 同一未缓存用户 → DB 仅 1 次命中；负缓存 30s 内二次 0 命中，>30s 再来 1 次。
- **B 幂等**：同 key 连发两次 → 一次 201、一次相同 body、`orders` 表仅 1 行；换 key → 新行；A 的 key 给 B → 各自成功（hash 含 userID）；停 Redis/不带 header → 退化为现状。
- **阶段0 限流**：单 IP 连发 101 次 → ~100 成功、余 429；`X-Forwarded-For: 1.2.3.4, 10.0.0.1` → 看到 `1.2.3.4`。

---

## ✅ 阶段 2：可观测性完善

1. **日志**：logx 切 JSON 输出；接入 **traceID/spanID 注入日志**（go-zero logx 天然支持）；GORM logger 从 `Info` 降到 `Warn` 且脱敏（当前 `xdb/db.go` 是 `logger.Info` 会打全量 SQL 含 PII）。
2. **日志聚合**：compose 加 **Loki + Promtail**，Grafana 统一面板。
3. **指标+告警**：补 Prometheus alert rules（5xx 率、p99 延迟、熔断 open、DB 连接饱和）；加业务指标（下单 QPS、注册成功数）。

### 实现细节

- **日志**：`common/xdb/gormlogger.go` 自定义 GORM logger，慢查询阈值 200ms（可通过 `DATABASE_SLOW_THRESHOLD` 环境变量调整），SQL 字面量脱敏（`'value'` → `'?'`），`gorm.ErrRecordNotFound` 不记为错误。
- **日志聚合**：`docker-compose.yml` 包含 Loki (`grafana/loki:3.5.5`) + Promtail (`grafana/promtail:3.5.5`)。Promtail 使用 Docker 服务发现（标签 `logging: promtail`），解析 JSON 日志提取 `level`/`service` 作为 Loki 标签。Grafana 预配置 Loki 数据源。
- **业务指标**：`common/xmetrics/xmetrics.go` 注册 `orders_created_total`、`users_registered_total`、`rpc_calls_breaker_open_total` 三个 Prometheus 计数器。
- **DB 连接池指标**：`common/xdb/db.go` 注册 `collectors.NewDBStatsCollector`，暴露 `go_sql_*` 系列指标。
- **告警规则**：`deploy/prometheus/rules.yml` 定义 4 条告警（HighErrorRate、HighLatencyP99、CircuitBreakerOpen、DBConnectionSaturation）。
- **Prometheus 采集**：user-api(:9101)、order-api(:9102)、user-rpc(:9103) 均配置独立 Prometheus 端口。

## ✅ 阶段 3：安全加固

4. **TLS**：HTTP 走 HTTPS；gRPC 启用 TLS；服务间可选 mTLS（或上 service mesh）。
5. **密钥管理**：去掉 `change-me-in-production` 默认 secret；接入 Vault / 云 KMS / 文件挂载；确认 `.gitignore` 无 etc 明文。
6. **CORS 收紧**：白名单 origin，按环境配置（当前 `cors.go` 是 `*`）。
7. **网关**：引入 APISIX/Traefik 做 TLS 终止、统一鉴权、全局限流。（待做）

### 实现细节

- **CORS 收紧**：`common/middleware/cors.go` 改为 `NewCorsMiddleware(cfg CORSConfig)`，支持白名单 origin。默认 `["*"]`（开发模式），生产通过 `CORS_ALLOWED_ORIGINS` 环境变量配置。非通配符时返回 `Vary: Origin` 头，正确处理 HTTP 缓存。
- **密钥校验**：`common/config/types.go` 提供 `ValidateSecrets(mode, jwtSecrets)` 函数。`dev`/`test` 模式输出警告；`pro`/`pre` 模式拒绝启动。user-api 和 order-api 在 `cfg.ApplyEnvOverrides()` 后调用。
- **HTTP TLS**：go-zero `RestConf` 已内置 `CertFile`/`KeyFile` 字段，通过 `TLS_CERT_FILE`/`TLS_KEY_FILE` 环境变量注入。未设置时退化为 HTTP（开发默认）。生产环境 TLS 由基础设施层（API Gateway / Ingress Controller / Service Mesh）统一终止。
- **gRPC TLS**：gRPC TLS 由基础设施层（Service Mesh mTLS）统一处理，应用代码不直接管理证书。
- **网关**：待后续阶段引入 APISIX/Traefik。

## ⬜ 阶段 4：部署与运维

8. **Kubernetes**：Helm chart + Deployment/Service/ConfigMap/Secret；liveness & readiness probe（复用 `/health`）。
9. **CI/CD**：GitHub Actions → golangci-lint → test → docker build/push → 部署。镜像用 distroless + 非 root（当前无 `.github/workflows`、无 golangci-lint 配置）。
10. **资源与弹性**：CPU/内存 limit + HPA（基于 QPS/CPU）。
11. **迁移分离**：迁移从启动逻辑拆成独立 Job/Init Container，避免多副本启动竞争（虽已有 advisory lock）。

## ⬜ 阶段 5：数据一致性与规模化

12. **Outbox 模式**：order 写库时同事务写 outbox 表，后台 worker 投递（为接 MQ/事件做准备）。
13. **异步化**：引入 Kafka/NATS 做事件解耦。
14. **只读副本 + 读写分离**；Redis 集群/哨兵；连接池按流量模型调优。

---

## 目标架构（最终形态）

```
                  ┌─────────────┐
   外部流量 ──TLS──▶│  API Gateway │  (APISIX/Traefik: TLS、限流、鉴权、路由)
                  └──────┬──────┘
            ┌────────────┼────────────┐
            ▼            ▼            ▼
       ┌────────┐   ┌────────┐   ┌────────┐
       │user-api│   │order-api│  │ ...(新服务)   x N
       └───┬────┘   └────┬────┘
           │ gRPC+mTLS    │
           ▼              ▼
       ┌────────┐   ┌────────┐
       │user-rpc│◀──┤ 熔断+重试 │  (go-zero 内置)
       │ x N    │   └────────┘
       └───┬────┘
           │
     ┌─────┴─────┬──────────┐
     ▼           ▼          ▼
 ┌────────┐ ┌────────┐ ┌────────┐
 │Postgres│ │ Redis  │ │ Kafka  │
 │主+只读  │ │集群    │ │(事件)  │
 └────────┘ └────────┘ └────────┘

  可观测平面：Jaeger(链路) + Prometheus+Loki(指标/日志) → Grafana → Alertmanager
```

## 约束偏好
- 学习目的、贴近真实生产；部署形态先 Docker 将来 K8s。
- 优先复用 go-zero 内置能力，少引第三方；Kafka/Vault/Service Mesh 等重基础设施按需引入。
- 保持当前 go-zero 标准目录与 `common/x*` 约定。

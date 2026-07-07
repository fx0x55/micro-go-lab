# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Run individual services (requires MySQL + etcd + redis running locally)
make run-user-api    # HTTP :8080
make run-user-rpc    # gRPC :9090
make run-order-api   # HTTP :8081

# Build all binaries to bin/
make build

# Run all tests
make test
# Single package test
go test ./service/user/api/internal/logic/... -v
# Single test
go test ./service/user/api/internal/logic/... -v -run TestName

# Full stack via Docker (etcd, mysql, redis, user-api, user-rpc, order-api)
make docker-up
# Full stack + monitoring (prometheus, grafana, jaeger, loki, promtail)
make docker-full
make docker-down

# Development mode: infra Docker + native Go (fastest iteration)
make infra                    # Start etcd/mysql/redis only
make dev-user-api             # Infra + local go run
make dev-user-rpc             # Infra + local go run
make dev-order-api            # Infra + local go run
make infra-full               # Infra + monitoring
make infra-down               # Stop all infra

# Container debug mode (Delve)
make debug                    # All services with Delve
make debug-user-api           # Single service with Delve (port 40001)
make debug-user-rpc           # Single service with Delve (port 40002)
make debug-order-api          # Single service with Delve (port 40003)

# Go proxy (required in China network)
GOPROXY=https://goproxy.cn go mod download

# Regenerate rpc code (pb/ + userservice/ + internal/server/) — logic/svc/config 保留
make proto
# Regenerate api code (types.go + routes.go) — handler/logic/svc/config 保留
make gen-api

# Database migrations run automatically at service startup (embedded goose).
# To add a migration: drop a new SQL file in common/xdb/migrations/{user,order}/
# named NNNNN_description.sql with `-- +goose Up` / `-- +goose Down` sections.
```

## Architecture

Three services — **user-api**, **user-rpc**, and **order-api** — built on the [go-zero](https://github.com/zeromicro/go-zero) framework following the go-zero standard monorepo layout.

### go-zero standard layout (per service)

```
service/{name}/{api|rpc}/
  ├── etc/                    → YAML config files
  ├── internal/
  │   ├── config/             → per-service Config struct
  │   ├── handler/            → thin HTTP handlers (api only)
  │   ├── logic/              → business logic (one file per endpoint/rpc method)
  │   ├── svc/                → ServiceContext (dependency injection container)
  │   ├── types/              → request/response structs (api only)
  │   ├── model/              → GORM model structs (if per-service only)
  │   └── repository/         → data access (GORM queries)
  ├── pb/                     → generated protobuf code (rpc only)
  └── {name}{api|rpc}.go      → main entry point
```

**Shared packages (common/):**
- `common/config` — shared config types (`DatabaseConfig`, `JWTConfig`, `UserSvcConfig`)
- `common/xdb` — GORM connection (`New`) + goose SQL migrations (`Migrate`, files embedded under `migrations/{user,order}/`)
- `common/middleware` — JSON response helpers (`OkJson`/`CreatedJson`/`BadRequest`/...), `GetUserID` (JWT context), `HealthHandler` (DB ping → 503 on failure)
- `common/client` — gRPC client wrapper (order-api → user-rpc) + exponential-backoff retry interceptor

- `common/validator` — `go-playground/validator` adapted to go-zero `httpx.SetValidator`

### Service interactions

- **user-api** is a pure HTTP gateway (register, login, profile) on :8080; it does NOT connect to any database — all user data access goes through user-rpc via gRPC
- **user-rpc** is the single owner of the user domain: gRPC (`CreateUser`, `Authenticate`, `ValidateUser`, `GetUser`) on :9090, registered with etcd as `user-svc.rpc`; it owns `users_db`, runs the user-events Outbox Poller, and exposes cache-aside via Redis
- **order-api** owns the order domain: REST (orders CRUD) on HTTP :8081, calls user-rpc via gRPC to validate users before creating orders; it owns `orders_db` and runs the order-events Outbox Poller
- Service discovery: user-rpc registers on etcd; user-api and order-api discover it via `discov.EtcdConf`
- **Service boundary**: `users_db` is only accessed by user-rpc; `orders_db` only by order-api. No service reads another service's database directly — cross-service data access is always via gRPC.

### Key conventions

- **ServiceContext pattern**: each service's `internal/svc/servicecontext.go` holds all dependencies (DB, repos, clients); replaces manual constructor wiring
- **Logic layer**: business logic lives in `internal/logic/`, one file per endpoint; handlers are thin adapters that create a Logic per request
- **JWT auth**: go-zero's built-in `rest.WithJwt(secret)` middleware; user ID extracted from context key `"user_id"`
- **Response format**: all endpoints use the shared `middleware.Response{Code, Message, Data}` wrapper — never write raw JSON
- **Config**: YAML files in `etc/` per service; secrets and deploy-specific values injected via `ApplyEnvOverrides` reading `os.Getenv`
- **Database**: MySQL via GORM; schema managed by **goose** SQL migrations (embedded via `go:embed`, run automatically at startup in `common/xdb/migrate.go`)
- **Proto source** in `api/user/v1/`; generated code in `service/user/rpc/pb/` (gitignored, regenerate with `make proto`)

### Background Goroutine Lifecycle

所有后台 goroutine 统一由 context + WaitGroup 管理，由 `ServiceContext.Stop()` 统一关闭。

**启动流程** (`main` → `NewServiceContext`):
```
ctx, cancel := context.WithCancel(context.Background())
svcCtx = NewServiceContext(ctx, cfg)
  → poller.Start(ctx, &wg)
  → consumer.Start(ctx, &wg)
  → ratelimiter = NewRateLimiter(ctx, ...)
```

**关闭流程** (`SIGINT` → `proc.AddShutdownListener`):
```
svcCtx.Stop()
  → cancel()    // ctx 取消，所有 goroutine 收到信号
  → wg.Wait()   // 阻塞直到每个 goroutine 调用 wg.Done()
```

**新增后台 goroutine 的规范**：
1. 函数签名：`func Start(ctx context.Context, wg *sync.WaitGroup)`
2. `Start()` 中调用 `wg.Add(1)`，启动 `go func(ctx, wg)`
3. goroutine 内部用 `select case <-ctx.Done()` 检测取消信号，退出前调用 `wg.Done()`
4. 循环体包裹 `xstream.RunWithRecover(ctx, caller, fn)` 防止单次 panic 杀死整个进程

**Panic 隔离** (`common/xstream/recover.go`):
`RunWithRecover(ctx, caller, fn)` — panic 时记录 caller 标识 + 堆栈，不传播，goroutine 继续下一轮循环。适用于所有长运行循环。

### Distributed tracing (OpenTelemetry → Jaeger)

Tracing is handled entirely by go-zero's built-in OTel integration — no custom middleware or interceptors needed.

- **TracerProvider lifecycle**: `ServiceConf.SetUp()` (called via `cfg.MustSetUp()` in each service's `main()`) initializes the global `TracerProvider` with an OTLP gRPC exporter and registers W3C TraceContext + Baggage propagators. Shutdown is registered via `proc.AddShutdownListener`.
- **HTTP spans**: go-zero's `TraceHandler` middleware is enabled by default (`Middlewares.Trace: true` in `RestConf`) — creates a server span per request, extracts/injects trace context via headers.
- **gRPC server spans**: go-zero's `UnaryTracingInterceptor` / `StreamTracingInterceptor` are enabled by default (`Middlewares.Trace: true` in `RpcServerConf`) — creates a server span per RPC, extracts context from gRPC metadata.
- **gRPC client spans**: go-zero's client `UnaryTracingInterceptor` is enabled by default (`Middlewares.Trace: true` in `RpcClientConf`) — creates a client span per outgoing call, injects context into gRPC metadata. The order-api → user-rpc calls automatically propagate trace context.
- **Config**: each service's YAML has a `Telemetry` section (go-zero's `trace.Config`):
  ```yaml
  Telemetry:
    Endpoint: localhost:4317  # OTLP gRPC collector (Jaeger)
    Sampler: 1.0              # 100% sampling
    Batcher: otlpgrpc         # exporter type
  ```
- **Env override**: `OTLP_ENDPOINT` env var maps to `cfg.Telemetry.Endpoint` via each service's `ApplyEnvOverrides()`. Docker compose sets `OTLP_ENDPOINT: jaeger:4317`.
- **Jaeger UI**: `localhost:16686` (docker). Search by service name: `user-api`, `user-rpc`, `order-api`.
- **Grafana**: Jaeger is provisioned as a datasource at `http://jaeger:16686`.

### Observability (Structured Logging + Metrics + Alerts)

#### Structured Logging

- **JSON output**: all services log in JSON format via logx (configured in each `etc/*.yaml` `Log` section with `Encoding: json`). This enables log aggregation tools to parse and index logs by fields.
- **Trace context injection**: `logx.WithContext(ctx)` automatically injects `trace` and `span` fields into JSON logs. All business logic uses `logx.WithContext(ctx)` for full trace correlation. Note: request-path logs (`common/middleware/logger.go`) use `logx.WithContext(r.Context())` and include HTTP-specific fields (`method`, `path`, `status`, `duration`, `ip`).
- **GORM SQL logging** (`common/xdb/gormlogger.go`): custom logger replaces `logger.Default.LogMode(logger.Info)`:
  - Default level **Warn** — only logs slow queries and errors, not all SQL (prevents PII exposure from full SQL dumps).
  - **Slow query threshold**: configurable via `DatabaseConfig.SlowThreshold` (default 200ms). Set `DATABASE_SLOW_THRESHOLD` env var to override.
  - **PII desensitization**: SQL string literals are redacted (`'value'` → `'?'`) before logging via regex `'(?:[^']|'')*'`.
  - **Error filtering**: `gorm.ErrRecordNotFound` is excluded from error-level logging (expected in many query flows).

#### Log Aggregation (Loki + Promtail)

Docker Compose includes:
- **Loki** (`grafana/loki:3.5.5`): log storage and query engine on port 3100, configured for single-node filesystem storage.
- **Promtail** (`grafana/promtail:3.5.5`): log collector using Docker service discovery (`docker_sd_configs`), automatically discovers containers labeled `logging: promtail`.
  - Pipeline stages: extracts `level` and `service` as Loki labels (avoids high-cardinality `trace` indexing), retains `trace`/`span` in log lines for full-text search.
- **Grafana**: Loki is provisioned as a datasource at `http://loki:3100`. Query examples:
  - `{service="user-api"} | json` — all JSON-structured logs from user-api
  - `{service="order-api"} | json | level="error"` — error logs from order-api
  - `{service=~".*"} | json | trace="<traceID>"` — correlate logs by trace ID across services

#### Business Metrics

Custom Prometheus counters registered in `common/xmetrics` (auto-registered via `init()`):

- **`orders_created_total`** (order-api): labels `{result}` where result ∈ {success, conflict, error}. Tracks order creation volume, conflicts from idempotency gate, and failures.
- **`users_registered_total`** (user-api): labels `{result}` where result ∈ {success, exists, error}. Tracks registration attempts.
- **`rpc_calls_breaker_open_total`** (order-api): labels `{method}`. Incremented when `ValidateUser`/`GetUser` gRPC calls are rejected by go-zero's circuit breaker (`breaker.ErrServiceUnavailable`). Enables alerting on breaker-open state.

#### DB Connection Pool Metrics

`common/xdb/db.go` registers `collectors.NewDBStatsCollector(sqlDB, cfg.DBName)` from `prometheus/client_golang`, exposing `go_sql_*` metrics (open connections, in-use, idle, wait count/duration) labeled by database name.

#### Prometheus Alert Rules

Alert definitions in `deploy/prometheus/rules.yml` (loaded by `deploy/prometheus.yml`):

| Alert | Condition | Severity | Description |
|---|---|---|---|
| `HighErrorRate` | 5xx rate > 5% for 5m | warning | HTTP error spike |
| `HighLatencyP99` | p99 latency > 1s for 5m | warning | Latency degradation |
| `CircuitBreakerOpen` | `rpc_calls_breaker_open_total` increase > 0 for 5m | critical | Downstream dependency failing |
| `DBConnectionSaturation` | `go_sql_in_use_connections / go_sql_max_open_connections > 0.8` for 5m | warning | Connection pool exhaustion |

Note: go-zero's HTTP metrics are `{http_server_requests_duration_ms, http_server_requests_code_total}` with labels `{path, method, code}`. Alerts aggregate across `job` label (service).

### Databases

Single MySQL instance with two databases (each owned by exactly one service):
- `users_db` — owned by **user-rpc** only: `users`, `outbox_events`, `processed_events`. user-api no longer connects here.
- `orders_db` — owned by **order-api** only: `orders`, `outbox_events`, `processed_events`. Order (Amount is int64, in cents/fen).

Each database is the private state of its owning service; cross-service access must go through gRPC.

### Environment Variables

New variables added in Phase 2 (Observability):

| Variable | Default | Description |
|---|---|---|
| `DATABASE_SLOW_THRESHOLD` | 200ms | SQL query duration threshold for slow query logging (e.g., "500ms") |

Note: pre-existing variables from Phases 0-1:
- `DATABASE_SSLMODE`, `DATABASE_CONN_MAX_LIFETIME`, `DATABASE_CONN_MAX_IDLE_TIME`
- `CACHE_TTL`, `CACHE_NEGATIVE_TTL`
- `REDIS_HOST` (order-api only), `JWT_SECRET`, `OTLP_ENDPOINT`, `ETCD_HOSTS`, `ETCD_KEY`, `DATABASE_HOST`, `DATABASE_PORT`

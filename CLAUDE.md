# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Run individual services (requires PostgreSQL + etcd running locally)
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

# Full stack via Docker (etcd, postgres x2, user-api, user-rpc, order-api, prometheus, grafana, jaeger)
make docker-up
make docker-down

# Go proxy (required in China network)
GOPROXY=https://goproxy.cn go mod download

# Regenerate protobuf (after editing api/user/v1/user.proto) — output lands in service/user/rpc/pb/
make proto

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
- `common/config` — shared config types (`DatabaseConfig`, `JWTConfig`, `TelemetryConfig`, `UserSvcConfig`)
- `common/xdb` — GORM connection (`New`) + goose SQL migrations (`Migrate`, files embedded under `migrations/{user,order}/`)
- `common/middleware` — JSON response helpers (`OkJson`/`CreatedJson`/`BadRequest`/...), `GetUserID` (JWT context), `HealthHandler` (DB ping → 503 on failure)
- `common/client` — gRPC client wrapper (order-api → user-rpc) + exponential-backoff retry interceptor
- `common/model` — shared GORM model structs (User, Todo, Order)
- `common/validator` — `go-playground/validator` adapted to go-zero `httpx.SetValidator`
- `common/telemetry` — OpenTelemetry init (OTLP → Jaeger)

### Service interactions

- **user-api** serves REST (register, login, profile, todos CRUD) on HTTP :8080
- **user-rpc** serves gRPC (`ValidateUser`, `GetUser`) on :9090, registered with etcd as `user-svc.rpc`
- **order-api** serves REST (orders CRUD) on HTTP :8081, calls user-rpc via gRPC to validate users before creating orders
- Service discovery: user-rpc registers on etcd; order-api discovers it via `discov.EtcdConf`

### Key conventions

- **ServiceContext pattern**: each service's `internal/svc/servicecontext.go` holds all dependencies (DB, repos, clients); replaces manual constructor wiring
- **Logic layer**: business logic lives in `internal/logic/`, one file per endpoint; handlers are thin adapters that create a Logic per request
- **JWT auth**: go-zero's built-in `rest.WithJwt(secret)` middleware; user ID extracted from context key `"user_id"`
- **Response format**: all endpoints use the shared `middleware.Response{Code, Message, Data}` wrapper — never write raw JSON
- **Config**: YAML files in `etc/` per service; secrets and deploy-specific values injected via `ApplyEnvOverrides` reading `os.Getenv`
- **Database**: PostgreSQL via GORM; schema managed by **goose** SQL migrations (embedded via `go:embed`, run automatically at startup in `common/xdb/migrate.go`)
- **Proto source** in `api/user/v1/`; generated code in `service/user/rpc/pb/` (gitignored, regenerate with `make proto`)

### Databases

- `users_db` (port 5432) — user-api/user-rpc models: User, Todo
- `orders_db` (port 5433) — order-api models: Order (Amount is int64, in cents/fen)

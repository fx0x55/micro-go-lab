# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Run individual services (requires PostgreSQL + etcd running locally)
make run-user       # HTTP :8080, gRPC :9090
make run-order      # HTTP :8081

# Build both binaries to bin/
make build

# Run all tests
make test
# Single package test
go test ./internal/user/service/... -v
# Single test
go test ./internal/user/service/... -v -run TestName

# Full stack via Docker (etcd, postgres x2, both services, prometheus, grafana, jaeger)
make docker-up
make docker-down

# Go proxy (required in China network)
GOPROXY=https://goproxy.cn go mod download

# Regenerate protobuf (after editing proto/ files) — output lands in gen/
protoc --go_out=. --go-grpc_out=. \
  --go_opt=module=github.com/wokoworks/go-server \
  --go-grpc_opt=module=github.com/wokoworks/go-server \
  proto/user/v1/user.proto

# Database migrations run automatically at service startup (embedded goose).
# To add a migration: drop a new SQL file in internal/db/migrations/{user,order}/
# named NNNNN_description.sql with `-- +goose Up` / `-- +goose Down` sections.
```

## Architecture

Two microservices — **user-svc** and **order-svc** — built on the [go-zero](https://github.com/zeromicro/go-zero) framework with gRPC inter-service communication and etcd service discovery.

### Layered structure (per service)

```
cmd/{svc}/main.go          → bootstrap, dependency wiring, graceful shutdown
cmd/{svc}/routes.go         → HTTP route registration (go-zero rest.Server)
internal/{svc}/handler/     → HTTP handlers (parse request, call service, respond)
internal/{svc}/service/     → business logic
internal/{svc}/repository/  → data access (GORM queries)
internal/{svc}/model/       → GORM model structs
```

**Shared packages:**
- `internal/config` — single `Config` struct loaded by both services via go-zero's `conf.MustLoad`; secrets/env via `ApplyEnvOverrides`
- `internal/db` — GORM connection (`New`) + goose SQL migrations (`Migrate`, files embedded under `migrations/{user,order}/`)
- `internal/middleware` — JSON response helpers (`OkJson`/`CreatedJson`/`BadRequest`/...), `GetUserID` (JWT context), `HealthHandler` (DB ping → 503 on failure)
- `internal/client` — gRPC client wrapper (order-svc → user-svc) + exponential-backoff retry interceptor
- `internal/validator` — `go-playground/validator` adapted to go-zero `httpx.SetValidator` (use `validate:` tags on request structs)
- `internal/telemetry` — OpenTelemetry init (OTLP → Jaeger), wired into both `main.go`s

### Service interactions

- **user-svc** serves REST (register, login, profile, todos CRUD) + gRPC (`ValidateUser`, `GetUser`)
- **order-svc** serves REST (orders CRUD) and calls user-svc via gRPC to validate users before creating orders
- Service discovery: user-svc registers on etcd; order-svc discovers it via `discov.EtcdConf`

### Key conventions

- **JWT auth**: go-zero's built-in `rest.WithJwt(secret)` middleware; user ID extracted from context key `"user_id"`
- **Response format**: all endpoints use the shared `middleware.Response{Code, Message, Data}` wrapper — never write raw JSON
- **Config**: YAML files in `config/` loaded into `internal/config.Config`; secrets and deploy-specific values (DB host, etcd hosts, JWT secret, OTLP endpoint) injected via `ApplyEnvOverrides` reading `os.Getenv` — go-zero's `${VAR}` substitution does not support defaults, so explicit env overrides are used
- **Database**: PostgreSQL via GORM; schema managed by **goose** SQL migrations (embedded via `go:embed`, run automatically at startup in `internal/db/migrate.go`) — not `AutoMigrate`
- **Dependency injection**: manual constructor wiring in each `main.go` — `NewRepo(db)` → `NewService(repo)` → `NewHandler(svc)`
- **Proto-generated code** lives in `gen/` (gitignored); proto source in `proto/`

### Databases

- `users_db` (port 5432) — user-svc models: User, Todo
- `orders_db` (port 5433) — order-svc models: Order (Amount is int64, in cents/fen)

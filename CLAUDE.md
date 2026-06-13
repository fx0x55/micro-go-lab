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

# Regenerate protobuf (after editing proto/ files)
protoc --go_out=gen --go-grpc_out=gen proto/user/v1/user.proto
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
- `internal/config` — single `Config` struct loaded by both services via go-zero's `conf.MustLoad`
- `internal/middleware` — standardized JSON response helpers (`OkJson`, `CreatedJson`, `BadRequest`, `Unauthorized`, `NotFound`, `InternalError`)
- `internal/client` — gRPC client wrapper (order-svc → user-svc)
- `internal/telemetry` — OpenTelemetry init (defined but not yet wired into main)

### Service interactions

- **user-svc** serves REST (register, login, profile, todos CRUD) + gRPC (`ValidateUser`, `GetUser`)
- **order-svc** serves REST (orders CRUD) and calls user-svc via gRPC to validate users before creating orders
- Service discovery: user-svc registers on etcd; order-svc discovers it via `discov.EtcdConf`

### Key conventions

- **JWT auth**: go-zero's built-in `rest.WithJwt(secret)` middleware; user ID extracted from context key `"user_id"`
- **Response format**: all endpoints use the shared `middleware.Response{Code, Message, Data}` wrapper — never write raw JSON
- **Config**: YAML files in `config/` loaded into `internal/config.Config`; env vars supported via go-zero YAML substitution
- **Database**: PostgreSQL via GORM; schema managed by `AutoMigrate()` at startup (no migration tool)
- **Dependency injection**: manual constructor wiring in each `main.go` — `NewRepo(db)` → `NewService(repo)` → `NewHandler(svc)`
- **Proto-generated code** lives in `gen/` (gitignored); proto source in `proto/`

### Databases

- `users_db` (port 5432) — user-svc models: User, Todo
- `orders_db` (port 5433) — order-svc models: Order (Amount is int64, in cents/fen)

# micro-go-lab

基于 [go-zero](https://github.com/zeromicro/go-zero) 框架的 Go 微服务项目，包含用户服务（HTTP + gRPC）、订单服务、完整的可观测性栈（Jaeger / Prometheus / Grafana / Loki）。

## 架构概览

```
                  ┌─────────────┐         ┌─────────────┐
                  │  user-api   │         │  order-api  │
                  │  HTTP :8080 │         │  HTTP :8081 │
                  │  纯网关     │         │ 订单域所有者│
                  └──────┬──────┘         └──────┬──────┘
                         │ gRPC (etcd)           │ gRPC (etcd)
                         │                       │
                         ▼                       ▼
                  ┌──────────────────────────────────┐
                  │  user-rpc  gRPC :9090            │
                  │  用户域唯一所有者                 │
                  │  CreateUser / Authenticate       │
                  │  ValidateUser / GetUser          │
                  └──────┬───────────────────────────┘
                         │
                ┌────────┴────────┐
                ▼                 ▼
          ┌──────────┐       ┌──────────┐
          │  MySQL   │       │  Redis   │
          │ users_db │       │ Stream + │
          │ (GORM)   │       │  cache   │
          └──────────┘       └──────────┘
```

**三个服务：**

| 服务 | 协议 | 端口 | 说明 |
|------|------|------|------|
| `user-api` | HTTP REST | :8080 | 纯网关：注册/登录/资料，全部转发 user-rpc，不连库 |
| `user-rpc` | gRPC | :9090 | 用户域唯一所有者：`CreateUser` / `Authenticate` / `ValidateUser` / `GetUser`，生产 user-events |
| `order-api` | HTTP REST | :8081 | 订单域所有者：订单 CRUD，调用 user-rpc 校验用户，生产 order-events |

**基础设施：**

| 组件 | 用途 |
|------|------|
| MySQL | 双库：`users_db`（用户）、`orders_db`（订单） |
| etcd | 服务注册与发现（user-rpc → user-api / order-api） |
| Redis | 缓存 / 消息队列（Redis Streams 事务性 Outbox） |

## 快速开始

### 前置条件

- Go 1.22+
- Docker & Docker Compose
- protoc（如需重新生成 protobuf）

### 方式一：全栈 Docker（最简单）

```bash
make docker-up          # 启动所有服务 + 基础设施
# 或带上监控栈
make docker-full        # + Prometheus / Grafana / Jaeger / Loki
```

### 方式二：本地开发（推荐日常开发）

```bash
make infra              # 仅启动 etcd + mysql + redis
make dev-user-api       # 本地 Go 运行 user-api（:8080）
make dev-user-rpc       # 本地 Go 运行 user-rpc（:9090）
make dev-order-api      # 本地 Go 运行 order-api（:8081）
```

### 方式三：直接运行（需手动启动基础设施）

```bash
make run-user-api       # HTTP :8080
make run-user-rpc       # gRPC :9090
make run-order-api      # HTTP :8081
```

### 停止所有服务

```bash
make docker-down        # 停止所有容器（含监控）
make infra-down         # 停止开发环境基础设施
```

## API 接口

### user-api（:8080）

| 方法 | 路径 | 说明 | 认证 |
|------|------|------|------|
| POST | `/api/user/register` | 注册 | 无 |
| POST | `/api/user/login` | 登录，返回 JWT | 无 |
| GET | `/api/user/profile` | 获取当前用户信息 | JWT |

### order-api（:8081）

| 方法 | 路径 | 说明 | 认证 |
|------|------|------|------|
| POST | `/api/order` | 创建订单（调用 user-rpc 校验用户） | JWT |
| GET | `/api/order/:id` | 查询订单 | JWT |
| GET | `/api/orders` | 订单列表 | JWT |

### user-rpc（:9090）

gRPC 方法（proto 定义在 `api/user/v1/user.proto`）：

- `CreateUser` — 创建用户（注册），明文密码入参，bcrypt 哈希不出域
- `Authenticate` — 校验用户名+密码，返回身份（防枚举）
- `ValidateUser` — 校验用户是否存在（带 cache-aside 缓存）
- `GetUser` — 获取用户信息

**服务边界**：user-api 和 order-api 均不直连 `users_db`，所有用户数据访问必须经过 user-rpc。

## 项目结构

```
micro-go-lab/
├── api/user/v1/              # Protobuf 定义
├── common/                   # 共享包
│   ├── config/               # 共享配置类型
│   ├── client/               # gRPC 客户端（含重试）
│   ├── middleware/            # HTTP 中间件（JWT、日志、健康检查）
│   ├── validator/            # 请求校验
│   ├── xdb/                  # GORM 连接 + goose 迁移
│   ├── xmetrics/             # Prometheus 自定义指标
│   └── xstream/              # Panic 恢复工具
├── service/
│   ├── user/api/             # user-api 服务（纯网关，不连库）
│   ├── user/rpc/             # user-rpc 服务（用户域所有者，生产 user-events）
│   │   └── pb/               # 生成的 protobuf 代码
│   └── order/api/            # order-api 服务（订单域所有者，生产 order-events）
├── deploy/                   # 部署配置
│   ├── prometheus.yml        # Prometheus 配置
│   ├── prometheus/rules.yml  # 告警规则
│   ├── grafana/              # Grafana 数据源 & Dashboard
│   ├── loki/                 # Loki 配置
│   └── promtail/             # Promtail 采集配置
├── docker-compose.yml        # 全栈编排（含监控）
├── docker-dev.yml            # 开发环境（仅基础设施）
└── Dockerfile                # 多阶段构建
```

## 开发

### 构建

```bash
make build                  # 编译到 bin/ 目录
make clean                  # 清理
```

### 测试

```bash
make test                   # 运行所有测试
go test ./service/user/api/internal/logic/... -v    # 单个包
go test ./service/user/api/internal/logic/... -v -run TestName  # 单个测试
```

### Lint

```bash
make lint                   # 运行 golangci-lint
make format                 # 自动修复
```

### Protobuf

编辑 `api/user/v1/user.proto` 后重新生成：

```bash
make proto
```

输出到 `service/user/rpc/pb/`（已 gitignore）。

### 调试

```bash
make debug                  # 所有服务 Delve 调试模式
make debug-user-api         # 单服务调试（端口 40001）
make debug-user-rpc         # 端口 40002
make debug-order-api        # 端口 40003
```

## 可观测性

启动监控栈后（`make docker-full` 或 `make infra-full`）：

| 组件 | 地址 | 用途 |
|------|------|------|
| Jaeger | http://localhost:16686 | 分布式链路追踪（OpenTelemetry → OTLP） |
| Prometheus | http://localhost:9091 | 指标采集与告警 |
| Grafana | http://localhost:3000（admin/admin） | 指标 / 日志统一查看 |
| Loki | localhost:3100 | 日志存储（Promtail 自动采集） |

**Grafana 数据源（自动配置）：**
- Prometheus → `http://prometheus:9090`
- Jaeger → `http://jaeger:16686`
- Loki → `http://loki:3100`

### 自定义指标

| 指标 | 服务 | 说明 |
|------|------|------|
| `orders_created_total{result}` | order-api | 订单创建（success/conflict/error） |
| `users_registered_total{result}` | user-api | 注册（success/exists/error） |
| `rpc_calls_breaker_open_total{method}` | order-api | gRPC 熔断器触发 |

### 告警规则

| 告警 | 条件 | 级别 |
|------|------|------|
| HighErrorRate | 5xx > 5% 持续 5m | warning |
| HighLatencyP99 | P99 > 1s 持续 5m | warning |
| CircuitBreakerOpen | 熔断器开启 > 5m | critical |
| DBConnectionSaturation | 连接池使用率 > 80% 持续 5m | warning |

## 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `DATABASE_HOST` | localhost | MySQL 地址（user-rpc / order-api） |
| `DATABASE_PORT` | 3306 | MySQL 端口（user-rpc / order-api） |
| `DATABASE_SSLMODE` | disable | SSL 模式 |
| `DATABASE_CONN_MAX_LIFETIME` | 5m | 连接最大存活时间 |
| `DATABASE_CONN_MAX_IDLE_TIME` | 3m | 空闲连接最大存活时间 |
| `DATABASE_SLOW_THRESHOLD` | 200ms | 慢查询阈值 |
| `JWT_SECRET` | dev-only-secret... | JWT 签名密钥 |
| `REDIS_HOST` | localhost | Redis 地址 |
| `ETCD_HOSTS` | localhost:2379 | etcd 地址 |
| `ETCD_KEY` | user-svc.rpc | etcd 服务注册 key |
| `OTLP_ENDPOINT` | localhost:4317 | OTLP gRPC collector（Jaeger） |

## CI/CD

GitHub Actions 工作流：

- `.github/workflows/ci.yml` — 测试 + lint
- `.github/workflows/docker.yml` — Docker 镜像构建

## License

MIT

# Go 微服务学习与实战计划

## 项目概述

通过实战构建一个迷你电商平台，系统学习 Go 微服务架构。每阶段完成后 commit，保持可追溯。

---

## 阶段一：Go 语言基础 [已完成]

> 用户已掌握，跳过。

---

## 阶段二：Web 开发与项目结构 [已完成]

- [x] Web 框架（路由分组、中间件、参数绑定）
- [x] GORM 数据层（PostgreSQL、连接池）
- [x] 分层架构（handler → service → repository → model）
- [x] 配置管理（YAML）
- [x] JWT 认证中间件
- [x] 统一响应封装
- [x] 优雅关闭
- [x] Docker Compose 编排

---

## 阶段三：微服务拆分 + gRPC 通信 [已完成]

- [x] Proto 定义（user.proto: ValidateUser/GetUser）
- [x] protoc 代码生成
- [x] 项目重构为 user-svc + order-svc 双服务
- [x] gRPC Server 实现（user-svc 暴露 gRPC :9090）
- [x] gRPC Client 封装（order-svc 调用 user-svc）
- [x] 独立数据库（users_db / orders_db）
- [x] Dockerfile 多目标构建

---

## 阶段四：微服务治理 [已完成]

**可观测性：**
- [x] **指标**：Prometheus metrics（go-zero 内置，`/metrics`）
- [x] **链路追踪**：OpenTelemetry OTLP → Jaeger 跨服务追踪
- [x] **日志**：go-zero logx 结构化日志

**弹性设计：**
- [x] **熔断**：go-zero zrpc 客户端内置 breakers 拦截器
- [x] **重试**：自定义 gRPC 指数退避重试拦截器（`internal/client/retry.go`）
- [ ] **限流**：待评估（go-zero 内置 `rest.WithMiddleware` + `ratelimit` 可直接接入）

**基础设施：**
- [x] 服务注册发现：etcd + go-zero discov
- [x] 健康检查：`/health` 含 DB Ping 探活，失败返回 503
- [x] Docker Compose 集成 Prometheus + Grafana + Jaeger

---

## 阶段四补充：go-zero 迁移与代码审查修复 [已完成]

迁移到 go-zero 框架后，对项目做了一次全面审查并修复了 11 项问题：

- [x] 请求参数校验：引入 `go-playground/validator` + `httpx.SetValidator` 适配（`validate:` 标签）
- [x] 越权漏洞：todo 的增删改查按 `user_id` 作用域过滤（消除 IDOR）
- [x] 订单状态机：`pending → paid/cancelled` 合法转换校验
- [x] 注册唯一约束：捕获 `pgconn.PgError 23505` 返回友好错误
- [x] gRPC 接口区分 NotFound 与 Internal，`ValidateUserResponse` 带回 username
- [x] gRPC 显式重试拦截器（指数退避，针对 Unavailable/DeadlineExceeded/ResourceExhausted）
- [x] 配置结构对齐 go-zero（`rest.RestConf` + `ServiceConf`，环境变量走 `ApplyEnvOverrides`）
- [x] JWT secret 走环境变量（`JWT_SECRET`），不落库不硬编码
- [x] 数据库迁移从 `AutoMigrate` 切换到 **goose**（显式 SQL，可回滚）
- [x] 重复代码抽取：`internal/db.New`、`middleware.GetUserID`、`middleware.HealthHandler`
- [x] 可观测全量接入：Prometheus 指标 + OTLP 链路追踪

---

## 阶段五：容器化与编排 [待开始]

- [ ] Kubernetes Deployment/Service/Ingress YAML
- [ ] ConfigMap / Secret 管理配置
- [ ] Helm Chart 打包
- [ ] CI/CD 流水线（GitHub Actions）

---

## 阶段六：进阶与实战 [待开始]

- [ ] API 网关（APISIX 或自建）
- [ ] 消息队列异步通信（NATS / Kafka）
- [ ] 事件驱动架构（Event Sourcing / CQRS）
- [ ] 分布式事务（Saga 模式）
- [ ] 安全加固（mTLS、RBAC）

---

## 技术栈

| 层面 | 技术 |
|------|------|
| 微服务框架 | go-zero（rest + zrpc） |
| ORM | GORM + PostgreSQL |
| RPC | gRPC + Protobuf |
| 服务发现 | etcd + go-zero discov |
| 配置 | go-zero conf（YAML + 环境变量覆盖） |
| 日志 | go-zero logx |
| 认证 | JWT HS256（golang-jwt，secret 走环境变量） |
| 参数校验 | go-playground/validator |
| 数据库迁移 | pressly/goose（嵌入式 SQL） |
| 可观测性 | Prometheus + Grafana + Jaeger（OpenTelemetry OTLP） |
| 弹性 | go-zero breakers + 自定义重试拦截器 |
| 容器 | Docker + Docker Compose |

## 项目结构

```
go-server/
├── proto/                  # gRPC 服务定义（user.proto）
├── gen/                    # protoc 生成代码（gitignored）
├── cmd/
│   ├── user-svc/           # 用户服务入口（main.go + routes.go）
│   └── order-svc/          # 订单服务入口（main.go + routes.go）
├── internal/
│   ├── user/               # user-svc 业务（handler/service/repository/model/grpc）
│   ├── order/              # order-svc 业务（handler/service/repository/model）
│   ├── client/             # gRPC 客户端封装（user.go + retry.go 重试拦截器）
│   ├── config/             # 共用 Config 结构 + ApplyEnvOverrides
│   ├── db/                 # 数据库连接 + goose 迁移（migrations/{user,order}/）
│   ├── middleware/         # 共用中间件（response/auth/health）
│   ├── validator/          # validator/v10 适配 httpx.SetValidator
│   └── telemetry/          # OpenTelemetry 初始化（OTLP → Jaeger）
├── deployments/
│   ├── prometheus.yml      # Prometheus 抓取配置
│   └── grafana/            # Grafana 数据源 provisioning
├── config/                 # YAML 配置文件（user-svc.yaml / order-svc.yaml）
├── Dockerfile              # 多目标构建
├── docker-compose.yml      # 编排（etcd + 2 DB + 2 Service + Prometheus + Grafana + Jaeger）
├── Makefile
└── PLAN.md                 # 项目学习计划
```

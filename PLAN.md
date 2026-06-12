# Go 微服务学习与实战计划

## 项目概述

通过实战构建一个迷你电商平台，系统学习 Go 微服务架构。每阶段完成后 commit，保持可追溯。

---

## 阶段一：Go 语言基础 [已完成]

> 用户已掌握，跳过。

---

## 阶段二：Web 开发与项目结构 [已完成]

**Commit:** `c89e481` - 初始化 Go 微服务项目基础设施

- [x] Gin 框架（路由分组、中间件、参数绑定）
- [x] GORM 数据层（PostgreSQL、连接池、AutoMigrate）
- [x] 分层架构（handler → service → repository → model）
- [x] Viper 配置管理
- [x] JWT 认证中间件
- [x] 统一响应封装
- [x] 优雅关闭（signal + Shutdown）
- [x] Docker Compose 编排

---

## 阶段三：微服务拆分 + gRPC 通信 [已完成]

**Commits:**
- `56b0f86` - 添加 user-svc 用户服务
- `13e028a` - 添加 order-svc 订单服务 + gRPC 服务间通信

- [x] Proto 定义（user.proto: ValidateUser/GetUser）
- [x] protoc 代码生成
- [x] 项目重构为 user-svc + order-svc 双服务
- [x] gRPC Server 实现（user-svc 暴露 gRPC :9090）
- [x] gRPC Client 封装（order-svc 调用 user-svc）
- [x] 独立数据库（users_db / orders_db）
- [x] Dockerfile 多目标构建

---

## 阶段四：微服务治理 [已完成]

**Commits:**
- `84521f3` - Prometheus HTTP 指标中间件
- `3610afc` - OpenTelemetry 链路追踪
- `82bb61c` - 熔断器保护 gRPC 调用
- `85c5365` - 令牌桶限流中间件
- `682268f` - gRPC 重试退避 + 健康检查
- `9519987` - Prometheus + Grafana + Jaeger 监控栈

- [x] **可观测性 - 日志**：结构化日志（Zap）
- [x] **可观测性 - 指标**：Prometheus metrics 暴露
- [x] **可观测性 - 链路追踪**：OpenTelemetry + Jaeger 跨服务追踪
- [x] **弹性设计 - 熔断**：gobreaker 熔断器保护 gRPC 调用
- [x] **弹性设计 - 限流**：令牌桶限流中间件
- [x] **弹性设计 - 重试**：gRPC 调用重试与指数退避
- [x] **健康检查**：/health 包含 DB 连接状态
- [x] Docker Compose 集成 Prometheus + Grafana + Jaeger

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
| HTTP 框架 | Gin |
| ORM | GORM + PostgreSQL |
| RPC | gRPC + Protobuf |
| 配置 | Viper (YAML) |
| 日志 | Zap |
| 认证 | JWT (golang-jwt) |
| 容器 | Docker + Docker Compose |
| 监控 | Prometheus + Grafana |
| 链路追踪 | OpenTelemetry + Jaeger |
| 熔断 | gobreaker |
| 限流 | x/time/rate |

## 项目结构

```
go-server/
├── proto/                  # gRPC 服务定义
├── cmd/
│   ├── user-svc/           # 用户服务入口 (:8080 REST, :9090 gRPC)
│   └── order-svc/          # 订单服务入口 (:8081 REST)
├── internal/
│   ├── user/               # user-svc 业务代码
│   ├── order/              # order-svc 业务代码
│   ├── client/             # gRPC 客户端封装（熔断 + 重试）
│   ├── config/             # 共用配置
│   ├── middleware/          # 共用中间件（JWT/CORS/日志/metrics/限流/熔断）
│   └── telemetry/          # OpenTelemetry 初始化
├── deployments/
│   ├── prometheus.yml      # Prometheus 抓取配置
│   └── grafana/            # Grafana 数据源 provisioning
├── config/                 # YAML 配置文件
├── Dockerfile              # 多目标构建
├── docker-compose.yml      # 编排（2 DB + 2 Service + Prometheus + Grafana + Jaeger）
├── Makefile
└── PLAN.md                 # 项目学习计划
```

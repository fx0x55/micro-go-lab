# ServiceDown

- 级别：critical
- 触发条件：`up == 0` 持续 1 分钟（Prometheus 抓取失败）

## 现象

Prometheus 连续 1 分钟无法从 `{job}` 的 `/metrics` 端口抓取指标。
Alertmanager 路由到 critical 接收器，值班收到即时通知。

## 影响

该服务的所有指标告警**同时失效**（没有数据=无法告警）。
这是最高优先级：其他告警的前提是"能抓到数据"。

## 排查步骤

1. 确认容器/进程是否存活
   ```bash
   docker compose ps {service}
   docker compose logs --tail=200 {service}
   ```
2. 若容器已退出：看退出码与日志，常见原因——
   - 数据库连不上（`failed to connect database`）-> 检查 `mysql` 容器健康
   - 迁移失败（`failed to migrate`）-> 检查 migrations 与 DB 可用性
   - 端口冲突 -> `lsof -i :{port}`
3. 若容器存活但抓取失败：从 Prometheus 容器内直连指标端口
   ```bash
   docker compose exec prometheus wget -qO- http://{service}:{metrics_port}/metrics
   ```
   返回连接拒绝 -> 服务监听地址不对；返回超时 -> 服务 hang 住。

## 缓解 / 恢复

- 容器崩溃：`docker compose up -d {service}` 重启
- 配置错误（端口/地址）：修 `etc/*.yaml` 的 `Prometheus.Host:Port` 后重启
- 依赖故障：优先恢复依赖（DB/etcd/Redis），服务通常自带重连

## 事后

- 服务挂了多久？为何超过 1 分钟才触发
- 是否该加 readiness probe 让编排层自动重启

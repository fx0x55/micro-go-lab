# DBConnectionSaturation

- 级别：warning
- 触发条件：连接池使用率 `in_use / max_open > 0.8` 持续 5 分钟

## 现象

数据库连接池接近耗尽，新请求可能排队甚至超时。

## 影响

请求延迟上升（等待连接），最终表现为 5xx / 熔断。

## 排查步骤

1. 是否有慢查询长时间占用连接
   - 日志里搜慢查询（`DATABASE_SLOW_THRESHOLD`，默认 200ms）
   - Grafana 看 `go_sql_in_use_connections` 与 `go_sql_wait_duration_total` 趋势
2. 是否连接泄漏（开连接没关）——检查未 defer Close 的 GORM 用法
3. 是否 QPS 真的很高，`MaxOpenConns` 设小了

## 缓解 / 恢复

- 慢查询：加索引 / 优化 SQL
- 连接数不足：调大 `MaxOpenConns`（注意 DB 侧 `max_connections` 上限）
  环境变量：`DATABASE_CONN_MAX_LIFETIME` 等

## 事后

- 连接池配置是否需要按实际负载重新调优
- 是否该加查询级超时防止单查询霸占连接

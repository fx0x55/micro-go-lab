# HighErrorRate

- 级别：warning
- 触发条件：`job:http_error_ratio:5m > 0.05`（5xx 占比 > 5%）持续 5 分钟

## 现象

某服务 5xx 错误率超过 5%。

## 影响

用户请求失败率上升；通常先于用户投诉暴露问题。

## 排查步骤

1. Grafana RED 面板：按 `path` 维度看哪个接口错误率最高
2. Loki 按错误聚合：
   ```
   {service="{job}"} | json | level="error"
   ```
3. 用 trace 串起来：从一条 5xx 的 `trace` 字段进 Jaeger，看完整调用链定位根因
4. 关联时间点：是否与最近的发布/配置变更重合

## 缓解 / 恢复

- 代码缺陷：回滚到上一稳定版本
- 依赖故障：恢复依赖（DB/RPC），见对应手册
- 流量突增（限流）：见 ratelimit 配置

## 事后

- 错误是否本可被更早发现（考虑缩短 for 时长或加 recording）
- 该接口是否需要更细粒度的告警

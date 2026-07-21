package xmetrics

import "github.com/prometheus/client_golang/prometheus"

// DBDeadlocks 统计数据库死锁/锁等待的重试情况。
//
//	label db      ── 数据库名（如 inventory_db / users_db）
//	label outcome ── retried（被 xdb.WithRetry 捕获并重试）/ exhausted（重试次数耗尽仍失败）
//
// 由 common/xdb.WithRetry 在捕获到 InnoDB 1213（死锁）/ 1205（锁等待超时）时自增。
// 它是"发现"链的主信号：指标一涨，就说明死锁正在真实发生。
var DBDeadlocks = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "db_deadlocks_total",
	Help: "Total database deadlocks/lock-waits detected by the retry wrapper, by db and outcome.",
}, []string{"db", "outcome"})

func init() {
	prometheus.MustRegister(DBDeadlocks)
}

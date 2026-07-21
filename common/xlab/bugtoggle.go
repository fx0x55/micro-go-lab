package xlab

import (
	"os"
	"strconv"
)

// BugEnabled 读取故障注入开关：对应环境变量值为 "1" 视为开启。
// 供各 troubleshooting lab（BUG_CPU / BUG_MEMLEAK / BUG_DB_DEADLOCK ...）共用，
// 避免每个 lab 各自重声明一份相同的开关读取逻辑。
func BugEnabled(env string) bool { return os.Getenv(env) == "1" }

// BugEnvInt 读取故障注入相关的整型参数，未设置或解析失败时返回 def。
func BugEnvInt(env string, def int) int {
	v := os.Getenv(env)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return def
	}
	return n
}

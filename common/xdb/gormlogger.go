package xdb

import (
	"context"
	"errors"
	"regexp"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// sqlStringLiteralRe 用于匹配 SQL 中的字符串字面量（'...'），用于脱敏处理。
var sqlStringLiteralRe = regexp.MustCompile(`'(?:[^']|'')*'`)

// gormLogxLogger 实现 gorm.io/gorm/logger.Interface，通过 logx 输出结构化日志。
// 默认仅记录 Warn 级别以上的日志（慢查询和错误），并脱敏 SQL 中的字符串字面量。
type gormLogxLogger struct {
	level         gormlogger.LogLevel
	slowThreshold time.Duration
}

// NewGormLogger 创建 GORM logger，默认记录 Warn 级别（慢查询阈值为 threshold）。
func NewGormLogger(threshold time.Duration) *gormLogxLogger {
	return &gormLogxLogger{
		level:         gormlogger.Warn,
		slowThreshold: threshold,
	}
}

func (g *gormLogxLogger) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	newLogger := *g
	newLogger.level = level
	return &newLogger
}

func (g *gormLogxLogger) Info(ctx context.Context, msg string, data ...any) {
	if g.level >= gormlogger.Info {
		logx.WithContext(ctx).Infof(msg, data...)
	}
}

func (g *gormLogxLogger) Warn(ctx context.Context, msg string, data ...any) {
	if g.level >= gormlogger.Warn {
		logx.WithContext(ctx).Slowf(msg, data...)
	}
}

func (g *gormLogxLogger) Error(ctx context.Context, msg string, data ...any) {
	if g.level >= gormlogger.Error {
		logx.WithContext(ctx).Errorf(msg, data...)
	}
}

func (g *gormLogxLogger) Trace(
	ctx context.Context,
	begin time.Time,
	fc func() (sql string, rowsAffected int64),
	err error,
) {
	if g.level <= gormlogger.Silent {
		return
	}

	elapsed := time.Since(begin)

	switch {
	case err != nil && g.level >= gormlogger.Error && !isRecordNotFound(err):
		sql, rows := fc()
		logx.WithContext(ctx).Errorw("slow/error sql",
			logx.Field("elapsed", elapsed.String()),
			logx.Field("rows", rows),
			logx.Field("sql", redactSQL(sql)),
			logx.Field("error", err.Error()),
		)
	case g.slowThreshold > 0 && elapsed > g.slowThreshold && g.level >= gormlogger.Warn:
		sql, rows := fc()
		logx.WithContext(ctx).Sloww("slow sql",
			logx.Field("elapsed", elapsed.String()),
			logx.Field("rows", rows),
			logx.Field("threshold", g.slowThreshold.String()),
			logx.Field("sql", redactSQL(sql)),
		)
	}
}

// redactSQL 脱敏 SQL 中的字符串字面量（'value'），替换为 '?'。
func redactSQL(sql string) string {
	return sqlStringLiteralRe.ReplaceAllString(sql, "'?'")
}

func isRecordNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}

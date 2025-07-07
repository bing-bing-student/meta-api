package loggers

import (
	"context"
	"time"

	"gorm.io/gorm/logger"
)

type CompositeLogger struct {
	FullLogger logger.Interface
	SlowLogger logger.Interface
}

func (c *CompositeLogger) LogMode(level logger.LogLevel) logger.Interface {
	c.FullLogger.LogMode(level)
	c.SlowLogger.LogMode(level)
	return c
}

func (c *CompositeLogger) Info(ctx context.Context, msg string, data ...interface{}) {
	c.FullLogger.Info(ctx, msg, data...)
	c.SlowLogger.Info(ctx, msg, data...)
}

func (c *CompositeLogger) Warn(ctx context.Context, msg string, data ...interface{}) {
	c.FullLogger.Warn(ctx, msg, data...)
	c.SlowLogger.Warn(ctx, msg, data...)
}

func (c *CompositeLogger) Error(ctx context.Context, msg string, data ...interface{}) {
	c.FullLogger.Error(ctx, msg, data...)
	c.SlowLogger.Error(ctx, msg, data...)
}

func (c *CompositeLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	// 全量日志记录所有SQL
	c.FullLogger.Trace(ctx, begin, fc, err)

	// 慢日志只记录超过阈值的SQL
	c.SlowLogger.Trace(ctx, begin, fc, err)
}

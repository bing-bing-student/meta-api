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
	return &CompositeLogger{
		FullLogger: c.FullLogger.LogMode(level),
		SlowLogger: c.SlowLogger.LogMode(level),
	}
}

func (c *CompositeLogger) Info(ctx context.Context, msg string, data ...any) {
	c.FullLogger.Info(ctx, msg, data...)
	c.SlowLogger.Info(ctx, msg, data...)
}

func (c *CompositeLogger) Warn(ctx context.Context, msg string, data ...any) {
	c.FullLogger.Warn(ctx, msg, data...)
	c.SlowLogger.Warn(ctx, msg, data...)
}

func (c *CompositeLogger) Error(ctx context.Context, msg string, data ...any) {
	c.FullLogger.Error(ctx, msg, data...)
	c.SlowLogger.Error(ctx, msg, data...)
}

func (c *CompositeLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	c.FullLogger.Trace(ctx, begin, fc, err)
	c.SlowLogger.Trace(ctx, begin, fc, err)
}

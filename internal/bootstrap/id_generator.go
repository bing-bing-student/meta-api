package bootstrap

import (
	"time"

	"github.com/sony/sonyflake"
	"go.uber.org/zap"

	"meta-api/internal/common/constants"
)

// initIDGenerator 初始化ID生成器
func initIDGenerator(logger *zap.Logger) *sonyflake.Sonyflake {
	startTime, err := time.Parse(constants.TimeLayoutToSecond, constants.StartTime)
	if err != nil {
		logger.Error("parse time error", zap.Error(err))
		return nil
	}

	if sf := sonyflake.NewSonyflake(sonyflake.Settings{StartTime: startTime}); sf != nil {
		return sf
	}
	return nil
}

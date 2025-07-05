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

	sf := sonyflake.NewSonyflake(sonyflake.Settings{StartTime: startTime})
	if sf == nil {
		logger.Error("sonyflake init error")
		return nil
	}
	return sf
}

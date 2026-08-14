package bootstrap

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/sony/sonyflake"
	"go.uber.org/zap"

	"meta-api/common/constants"
	"meta-api/common/env"
)

// initIDGenerator 初始化ID生成器
func initIDGenerator(logger *zap.Logger) (*sonyflake.Sonyflake, error) {
	startTime, err := time.Parse(constants.TimeLayoutToSecond, constants.StartTime)
	if err != nil {
		return nil, fmt.Errorf("parse sonyflake start time: %w", err)
	}

	machineID, err := sonyflakeMachineIDFromEnv()
	if err != nil {
		return nil, err
	}

	logger.Info("sonyflake machine id configured", zap.Uint16("machine_id", machineID))

	sf, err := sonyflake.New(sonyflake.Settings{
		StartTime: startTime,
		MachineID: fixedSonyflakeMachineID(machineID),
	})
	if err != nil {
		return nil, fmt.Errorf("init sonyflake: %w", err)
	}
	return sf, nil
}

func sonyflakeMachineIDFromEnv() (uint16, error) {
	raw := strings.TrimSpace(os.Getenv(env.SonyflakeMachineID))
	if raw == "" {
		return 0, fmt.Errorf("missing required environment variable: %s", env.SonyflakeMachineID)
	}

	id, err := strconv.ParseUint(raw, 10, 16)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", env.SonyflakeMachineID, err)
	}
	return uint16(id), nil
}

func fixedSonyflakeMachineID(id uint16) func() (uint16, error) {
	return func() (uint16, error) {
		return id, nil
	}
}

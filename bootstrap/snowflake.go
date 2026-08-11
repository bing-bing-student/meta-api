package bootstrap

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/sony/sonyflake"
	"go.uber.org/zap"

	"meta-api/common/constants"
	"meta-api/common/utils"
)

const (
	envSonyflakeMachineID = "SONYFLAKE_MACHINE_ID"
	localMachineID        = uint16(1)
)

// initIDGenerator 初始化ID生成器
func initIDGenerator(logger *zap.Logger) (*sonyflake.Sonyflake, error) {
	startTime, err := time.Parse(constants.TimeLayoutToSecond, constants.StartTime)
	if err != nil {
		return nil, fmt.Errorf("parse sonyflake start time: %w", err)
	}

	settings := sonyflake.Settings{StartTime: startTime}
	machineID, ok, envErr := sonyflakeMachineIDFromEnv()
	if envErr != nil {
		return nil, envErr
	}
	if ok {
		settings.MachineID = fixedSonyflakeMachineID(machineID)
	}

	sf, err := sonyflake.New(settings)
	if err == nil {
		return sf, nil
	}
	if errors.Is(err, sonyflake.ErrNoPrivateAddress) && !utils.IsProductionEnv() {
		logger.Warn("sonyflake private ip missing, fallback to local machine id", zap.Uint16("machine_id", localMachineID), zap.Error(err))
		return sonyflake.New(sonyflake.Settings{
			StartTime: startTime,
			MachineID: fixedSonyflakeMachineID(localMachineID),
		})
	}

	return nil, fmt.Errorf("init sonyflake: %w", err)
}

func sonyflakeMachineIDFromEnv() (uint16, bool, error) {
	raw := strings.TrimSpace(os.Getenv(envSonyflakeMachineID))
	if raw == "" {
		return 0, false, nil
	}

	id, err := strconv.ParseUint(raw, 10, 16)
	if err != nil {
		return 0, false, fmt.Errorf("invalid %s: %w", envSonyflakeMachineID, err)
	}
	return uint16(id), true, nil
}

func fixedSonyflakeMachineID(id uint16) func() (uint16, error) {
	return func() (uint16, error) {
		return id, nil
	}
}

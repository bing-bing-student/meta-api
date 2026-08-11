package bootstrap

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"

	"meta-api/config"
)

// initLog 日志初始化
func initLog(config *config.LogConfig) (*zap.Logger, error) {
	if config == nil {
		return nil, fmt.Errorf("log config is nil")
	}
	if err := prepareLogFiles(config); err != nil {
		return nil, err
	}

	infoLogWriter := GetLogWriter(config, config.HTTPInfoLog)
	warnLogWriter := GetLogWriter(config, config.HTTPWarnLog)
	errLogWriter := GetLogWriter(config, config.HTTPErrLog)

	infoLevel := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
		return lvl == zapcore.InfoLevel
	})
	warnLevel := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
		return lvl == zapcore.WarnLevel
	})
	errorLevel := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
		return lvl == zapcore.ErrorLevel
	})

	infoCore := zapcore.NewCore(getInfoLevelEncoder(), infoLogWriter, infoLevel)
	warnCore := zapcore.NewCore(getWarnLevelEncoder(), warnLogWriter, warnLevel)
	errCore := zapcore.NewCore(getErrorLevelEncoder(), errLogWriter, errorLevel)

	core := zapcore.NewTee(infoCore, warnCore, errCore)
	logger := zap.New(core, zap.AddCaller())
	return logger, nil
}

func prepareLogFiles(config *config.LogConfig) error {
	paths := []string{
		config.MySQLFullLog,
		config.MySQLSlowLog,
		config.HTTPInfoLog,
		config.HTTPWarnLog,
		config.HTTPErrLog,
	}
	for _, path := range paths {
		if err := prepareLogFile(path); err != nil {
			return err
		}
	}
	return nil
}

func prepareLogFile(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("log path is empty")
	}

	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0750); err != nil {
			return fmt.Errorf("create log dir %q: %w", dir, err)
		}
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("open log file %q: %w", path, err)
	}
	if err = file.Close(); err != nil {
		return fmt.Errorf("close log file %q: %w", path, err)
	}
	return nil
}

// getInfoLevelEncoder 获取INFO日志的编码器
func getInfoLevelEncoder() zapcore.Encoder {
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "ts",
		LevelKey:       "level",
		MessageKey:     "msg",
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeLevel:    zapcore.CapitalLevelEncoder,
	}
	return zapcore.NewJSONEncoder(encoderConfig)
}

// getWarnLevelEncoder 获取WARN日志的编码器
func getWarnLevelEncoder() zapcore.Encoder {
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "ts",
		LevelKey:       "level",
		MessageKey:     "msg",
		CallerKey:      "caller",
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeCaller: func(caller zapcore.EntryCaller, enc zapcore.PrimitiveArrayEncoder) {
			rootPath, err := os.Getwd()
			if err != nil {
				panic("Get root path error: " + err.Error())
			}
			enc.AppendString(strings.TrimPrefix(caller.File, rootPath))
		},
	}
	return zapcore.NewJSONEncoder(encoderConfig)
}

// getErrorLevelEncoder 获取ERROR日志的编码器
func getErrorLevelEncoder() zapcore.Encoder {
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "ts",
		LevelKey:       "level",
		MessageKey:     "msg",
		CallerKey:      "caller",
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeCaller: func(caller zapcore.EntryCaller, enc zapcore.PrimitiveArrayEncoder) {
			rootPath, err := os.Getwd()
			if err != nil {
				panic("Get root path error: " + err.Error())
			}
			enc.AppendString(strings.TrimPrefix(caller.File, rootPath))
		},
	}
	return zapcore.NewJSONEncoder(encoderConfig)
}

// GetLogWriter 获取日志切割Writer
func GetLogWriter(config *config.LogConfig, path string) zapcore.WriteSyncer {
	// 使用 Lumberjack 实现日志切割
	lumberJackLogger := &lumberjack.Logger{
		Filename:   path,
		MaxSize:    config.MaxSize,    // 从配置读取 (单位: MB)
		MaxBackups: config.MaxBackups, // 从配置读取
		MaxAge:     config.MaxAge,     // 从配置读取 (单位: 天)
		Compress:   config.Compress,   // 从配置读取
	}

	// 创建文件时设置权限 (0600)
	//file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	//if err != nil {
	//	return zapcore.AddSync(os.Stderr)
	//}
	//defer func(file *os.File) {
	//	_ = file.Close()
	//}(file)

	return zapcore.AddSync(lumberJackLogger)
}

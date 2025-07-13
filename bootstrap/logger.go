package bootstrap

import (
	"log"
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"

	"meta-api/config"
)

// initLog 日志初始化
func initLog(config *config.LogConfig) *zap.Logger {
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
	return logger
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

	// 创建文件时设置严格权限 (0600)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		log.Printf("failed to open log file: %v", err)
		return zapcore.AddSync(os.Stderr)
	}
	defer func(file *os.File) {
		_ = file.Close()
	}(file)

	return zapcore.AddSync(lumberJackLogger)
}

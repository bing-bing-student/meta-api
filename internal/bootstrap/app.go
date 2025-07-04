package bootstrap

import (
	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/sony/sonyflake"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"meta-api/config"
)

// init 初始化环境变量
func init() {
	// 加载 .env 文件
	if err := godotenv.Load(); err != nil {
		return
	}

	// 加载 Docker Secrets
	files, err := os.ReadDir("/run/secrets")
	if err != nil {
		return
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		secretPath := "/run/secrets/" + file.Name()
		content, err := os.ReadFile(secretPath)
		if err != nil {
			continue
		}

		// 将文件名转为大写作为环境变量名
		envName := strings.ToUpper(file.Name())
		if err = os.Setenv(envName, strings.TrimSpace(string(content))); err != nil {
			return
		}
	}
}

// Application 应用程序
type Application struct {
	Config      *config.Config       // 配置
	Logger      *zap.Logger          // 日志
	IDGenerator *sonyflake.Sonyflake // 雪花ID生成器
	MySQL       *gorm.DB             // MySQL 客户端
	Redis       *redis.Client        // Redis 客户端
}

// New 创建应用程序
func New() *Application {
	return &Application{}
}

// InitConfig 初始化配置
func (app *Application) InitConfig() *Application {
	app.Config = initConfig()
	return app
}

// InitLogger 初始化日志
func (app *Application) InitLogger() *Application {
	app.Logger = initLog(app.Config.LogConfig)
	return app
}

// InitIDGenerator 初始化雪花ID生成器
func (app *Application) InitIDGenerator() *Application {
	app.IDGenerator = initIDGenerator(app.Logger)
	return app
}

// InitMySQL 创建MySQL客户端
func (app *Application) InitMySQL() *Application {
	mySQLConfig := &MySQLConfig{
		MySQLConfig: app.Config.MySQLConfig,
		LogConfig:   app.Config.LogConfig,
		RetryConfig: app.Config.RetryConfig,
	}

	app.MySQL = initMySQL(mySQLConfig)
	return app
}

// InitRedis 创建Redis客户端
func (app *Application) InitRedis() *Application {
	redisConfig := &RedisConfig{
		RedisConfig: app.Config.RedisConfig,
		RetryConfig: app.Config.RetryConfig,
	}
	app.Redis = initRedis(redisConfig)
	return app
}

// Start 启动所有服务组件
func (app *Application) Start() {}

// Stop 停止所有服务组件
func (app *Application) Stop() {
	if sqlDB, err := app.MySQL.DB(); err == nil {
		if err = sqlDB.Close(); err != nil {
			app.Logger.Error("failed to close MySQL connection", zap.Error(err))
		}
	} else {
		app.Logger.Error("failed to get MySQL DB instance", zap.Error(err))
	}

	if app.Redis != nil {
		if err := app.Redis.Close(); err != nil {
			app.Logger.Error("failed to close Redis connection", zap.Error(err))
		}
	}
}

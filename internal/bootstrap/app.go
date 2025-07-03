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

// Application 应用程序
type Application struct {
	Config      *config.Config       // 配置
	Logger      *zap.Logger          // 日志
	IDGenerator *sonyflake.Sonyflake // 雪花ID生成器
	MySQL       *gorm.DB             // MySQL 客户端
	Redis       *redis.Client        // Redis 客户端
}

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

// New 创建应用程序
func New() *Application {
	app := &Application{}

	app.Config = initConfig()                  // 初始化配置
	app.Logger = initLog(app.Config.LogConfig) // 初始化日志

	app.IDGenerator = initIDGenerator(app.Logger)                       // 初始化ID生成器
	app.MySQL = initMySQL(app.Config.MySQLConfig, app.Config.LogConfig) // 初始化MySQL
	app.Redis = initRedis(app.Config.RedisConfig, app.Logger)           // 初始化Redis

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

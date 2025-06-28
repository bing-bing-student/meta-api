package bootstrap

import (
	"github.com/redis/go-redis/v9"
	"github.com/sony/sonyflake"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"meta-api/config"
)

type SessionKeys struct {
	Authorization []byte // session 授权密钥
	Encryption    []byte // session 加密密钥
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
	app := &Application{}

	app.Config = initConfig()                                           // 初始化配置
	app.Logger = initLog(app.Config.LogConfig)                          // 初始化日志
	app.IDGenerator = initIDGenerator(app.Logger)                       // 初始化ID生成器
	app.MySQL = initMySQL(app.Config.MySQLConfig, app.Config.LogConfig) // 初始化MySQL
	app.Redis = initRedis(app.Config.RedisConfig, app.Logger)           // 初始化Redis

	return app
}

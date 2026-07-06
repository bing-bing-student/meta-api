package bootstrap

import (
	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/robfig/cron/v3"
	"github.com/sony/sonyflake"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"meta-api/config"
)

// init 初始化环境变量
func init() {
	// 加载本地 .env 文件；生产镜像允许不存在，不能阻断 Docker Secrets。
	_ = godotenv.Load()

	// 加载 Docker Secrets
	files, err := os.ReadDir("/run/secrets")
	if err != nil {
		return
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		content, err := os.ReadFile("/run/secrets/" + file.Name())
		if err != nil {
			continue
		}

		// 将文件名转为大写作为环境变量名
		if err = os.Setenv(strings.ToUpper(file.Name()), strings.TrimSpace(string(content))); err != nil {
			return
		}
	}
}

// Bootstrap 应用程序
type Bootstrap struct {
	Config          *config.Config       // 配置
	Logger          *zap.Logger          // 日志
	IDGenerator     *sonyflake.Sonyflake // 雪花 ID 生成器
	Cron            *cron.Cron           // 定时任务
	CronEntryIDList *[]cron.EntryID      // 定时任务 ID 列表
	MySQL           *gorm.DB             // MySQL 客户端
	Redis           *redis.Client        // Redis 客户端
}

// New 创建应用程序
func New() *Bootstrap {
	return &Bootstrap{}
}

// InitConfig 初始化配置
func (b *Bootstrap) InitConfig() *Bootstrap {
	b.Config = initConfig()
	return b
}

// InitLogger 初始化日志
func (b *Bootstrap) InitLogger() *Bootstrap {
	b.Logger = initLog(b.Config.LogConfig)
	return b
}

// InitIDGenerator 初始化雪花ID生成器
func (b *Bootstrap) InitIDGenerator() *Bootstrap {
	b.IDGenerator = initIDGenerator(b.Logger)
	return b
}

// InitCron 创建定时任务
func (b *Bootstrap) InitCron() *Bootstrap {
	b.Cron = InitCron()
	return b
}

// InitMySQL 创建MySQL客户端
func (b *Bootstrap) InitMySQL() *Bootstrap {
	mySQLConfig := &MySQLConfig{
		MySQLConfig: b.Config.MySQLConfig,
		LogConfig:   b.Config.LogConfig,
		RetryConfig: b.Config.RetryConfig,
	}

	b.MySQL = initMySQL(mySQLConfig)
	return b
}

// InitRedis 创建Redis客户端
func (b *Bootstrap) InitRedis() *Bootstrap {
	redisConfig := &RedisConfig{
		RedisConfig: b.Config.RedisConfig,
		RetryConfig: b.Config.RetryConfig,
	}
	b.Redis = initRedis(redisConfig)
	return b
}

// Start 启动所有服务组件
// 业务定时任务由 app 层 / service 层各自注册到 b.Cron 后再调用本方法启动调度器
func (b *Bootstrap) Start() {
	b.Cron.Start()
}

// Stop 停止所有服务组件
func (b *Bootstrap) Stop() {
	// 关闭定时任务
	if b.CronEntryIDList != nil {
		for _, entryID := range *b.CronEntryIDList {
			b.Cron.Remove(entryID)
		}
	}
	b.Cron.Stop()

	// 关闭 MySQL 数据库连接
	if sqlDB, err := b.MySQL.DB(); err == nil {
		if err = sqlDB.Close(); err != nil {
			b.Logger.Error("failed to close MySQL connection", zap.Error(err))
		}
	} else {
		b.Logger.Error("failed to get MySQL DB instance", zap.Error(err))
	}

	// 关闭 Redis 连接
	if b.Redis != nil {
		if err := b.Redis.Close(); err != nil {
			b.Logger.Error("failed to close Redis connection", zap.Error(err))
		}
	}
}

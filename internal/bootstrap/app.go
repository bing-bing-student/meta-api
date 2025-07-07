package bootstrap

import (
	"context"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/robfig/cron/v3"
	"github.com/sony/sonyflake"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"meta-api/config"
	"meta-api/internal/app/model"
	"meta-api/internal/common/constants"
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
	IDGenerator     *sonyflake.Sonyflake // 雪花ID生成器
	Cron            *cron.Cron           // 定时任务
	CronEntryIDList *[]cron.EntryID      // 定时任务ID列表
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
func (b *Bootstrap) Start(ctx context.Context) {
	// 定时任务配置并启动
	entryID, err := b.Cron.AddFunc(constants.Spec, func() {
		list, err := b.Redis.ZRangeWithScores(ctx, "article:view:ZSet", 0, -1).Result()
		if err != nil {
			b.Logger.Error("failed to query article:view:ZSet", zap.Error(err))
			return
		}
		for _, element := range list {
			articleID := element.Member.(string)
			viewNum := int(element.Score)
			if err = b.MySQL.Model(&model.Article{}).Where("id = ?", articleID).Update("view_num", viewNum).Error; err != nil {
				b.Logger.Error("failed to update article view num", zap.Error(err))
				return
			}
		}
	})
	if err != nil {
		b.Logger.Error("failed to add a scheduled task", zap.Error(err))
		return
	}

	b.CronEntryIDList = &[]cron.EntryID{entryID}
	b.Cron.Start()
}

// Stop 停止所有服务组件
func (b *Bootstrap) Stop() {
	// 关闭定时任务
	for _, entryID := range *b.CronEntryIDList {
		b.Cron.Remove(entryID)
	}
	b.Cron.Stop()

	// 关闭MySQL数据库连接
	if sqlDB, err := b.MySQL.DB(); err == nil {
		if err = sqlDB.Close(); err != nil {
			b.Logger.Error("failed to close MySQL connection", zap.Error(err))
		}
	} else {
		b.Logger.Error("failed to get MySQL DB instance", zap.Error(err))
	}

	// 关闭Redis连接
	if b.Redis != nil {
		if err := b.Redis.Close(); err != nil {
			b.Logger.Error("failed to close Redis connection", zap.Error(err))
		}
	}
}

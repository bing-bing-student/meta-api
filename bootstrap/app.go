package bootstrap

import (
	"context"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/robfig/cron/v3"
	"github.com/sony/sonyflake"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"meta-api/common/guard/keymanager"
	"meta-api/config"
)

// Bootstrap 应用程序
type Bootstrap struct {
	Config          *config.Config       // 配置
	RuntimeEnv      RuntimeEnv           // 启动期标准化后的运行时环境变量
	Logger          *zap.Logger          // 日志
	IDGenerator     *sonyflake.Sonyflake // 雪花 ID 生成器
	Cron            *cron.Cron           // 定时任务
	CronEntryIDList *[]cron.EntryID      // 定时任务 ID 列表
	MySQL           *gorm.DB             // MySQL 客户端
	Redis           *redis.Client        // Redis 客户端
	KeyManager      *keymanager.Manager  // 密钥管理器
	configWatcher   *ConfigWatcher       // 配置文件监听器
	lifecycleCtx    context.Context      // 生命周期进程级上下文
	lifecycleCancel context.CancelFunc   // 生命周期进程级取消函数
}

// New 创建应用程序
func New(runtimeEnv ...*RuntimeEnv) *Bootstrap {
	ctx, cancel := context.WithCancel(context.Background())
	env := defaultRuntimeEnv()
	if len(runtimeEnv) > 0 && runtimeEnv[0] != nil {
		env = *runtimeEnv[0]
	}
	return &Bootstrap{
		RuntimeEnv:      env,
		lifecycleCtx:    ctx,
		lifecycleCancel: cancel,
	}
}

// Context 返回进程级生命周期上下文。
func (b *Bootstrap) Context() context.Context {
	if b.lifecycleCtx == nil {
		return context.Background()
	}
	return b.lifecycleCtx
}

// InitConfig 初始化配置
func (b *Bootstrap) InitConfig() *Bootstrap {
	cfg, watcher, err := initConfig()
	if err != nil {
		log.Panicf("Read config files error: %v", err)
	}
	b.Config = cfg
	b.configWatcher = watcher
	return b
}

// InitLogger 初始化日志
func (b *Bootstrap) InitLogger() *Bootstrap {
	logger, err := initLog(b.Config.LogConfig)
	if err != nil {
		log.Panicf("init logger failed: %v", err)
	}
	b.Logger = logger
	return b
}

// InitIDGenerator 初始化雪花ID生成器
func (b *Bootstrap) InitIDGenerator() *Bootstrap {
	idGenerator, err := initIDGenerator(b.Logger)
	if err != nil {
		b.Logger.Panic("init id generator failed", zap.Error(err))
	}
	b.IDGenerator = idGenerator
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

	mySQL, err := initMySQL(mySQLConfig)
	if err != nil {
		b.Logger.Panic("init mysql failed", zap.Error(err))
	}
	b.MySQL = mySQL
	if err := autoMigrateMySQL(b.MySQL); err != nil {
		b.Logger.Panic("auto migrate mysql failed", zap.Error(err))
	}
	return b
}

// InitRedis 创建Redis客户端
func (b *Bootstrap) InitRedis() *Bootstrap {
	redisConfig := &RedisConfig{
		RedisConfig: b.Config.RedisConfig,
		RetryConfig: b.Config.RetryConfig,
	}
	redisClient, err := initRedis(redisConfig)
	if err != nil {
		b.Logger.Panic("init redis failed", zap.Error(err))
	}
	b.Redis = redisClient
	return b
}

// InitKeyManager 创建密钥管理器
func (b *Bootstrap) InitKeyManager() *Bootstrap {
	keyManager, err := keymanager.New(b.Logger)
	if err != nil {
		b.Logger.Panic("init key manager failed", zap.Error(err))
	}
	b.KeyManager = keyManager
	return b
}

// Start 启动所有服务组件
// 业务定时任务由 app 层 / service 层各自注册到 b.Cron 后再调用本方法启动调度器
func (b *Bootstrap) Start() {
	if b.Cron == nil {
		return
	}
	b.Cron.Start()
}

// Stop 停止所有服务组件
func (b *Bootstrap) Stop() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	b.StopCron(ctx)
	b.CloseResources()
}

// StopCron 停止定时任务调度器
func (b *Bootstrap) StopCron(ctx context.Context) {
	if b.Cron == nil {
		return
	}
	if b.CronEntryIDList != nil {
		for _, entryID := range *b.CronEntryIDList {
			b.Cron.Remove(entryID)
		}
	}
	stopCtx := b.Cron.Stop()
	select {
	case <-stopCtx.Done():
	case <-ctx.Done():
		if b.Logger != nil {
			b.Logger.Error("cron shutdown timed out", zap.Error(ctx.Err()))
		}
	}
}

// CloseResources 关闭基础资源连接
func (b *Bootstrap) CloseResources() {
	if b.lifecycleCancel != nil {
		b.lifecycleCancel()
	}

	// 关闭配置文件监听，释放 fsnotify fd
	if b.configWatcher != nil {
		if err := b.configWatcher.Close(); err != nil {
			b.logCloseError("failed to close config watcher", err)
		}
	}

	// 关闭 KeyManager 文件监听，释放 fsnotify fd
	if b.KeyManager != nil {
		if err := b.KeyManager.Close(); err != nil {
			b.logCloseError("failed to close keyManager", err)
		}
	}

	// 关闭 MySQL 数据库连接
	if b.MySQL != nil {
		if sqlDB, err := b.MySQL.DB(); err == nil {
			if err = sqlDB.Close(); err != nil {
				b.logCloseError("failed to close MySQL connection", err)
			}
		} else {
			b.logCloseError("failed to get MySQL DB instance", err)
		}
	}

	// 关闭 Redis 连接
	if b.Redis != nil {
		if err := b.Redis.Close(); err != nil {
			b.logCloseError("failed to close Redis connection", err)
		}
	}

	if b.Logger != nil {
		if err := b.Logger.Sync(); err != nil {
			b.logCloseError("failed to sync logger", err)
		}
	}
}

func (b *Bootstrap) logCloseError(message string, err error) {
	if b.Logger != nil {
		b.Logger.Error(message, zap.Error(err))
	}
}

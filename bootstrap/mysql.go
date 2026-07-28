package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"

	"meta-api/common/loggers"
	"meta-api/common/utils"
	"meta-api/config"
)

// ConnectMySQLClient 初始化MySQL客户端
func ConnectMySQLClient(ctx context.Context, config mysql.Config, logger logger.Interface, cfg *config.MySQLConfig) (*gorm.DB, error) {
	db, err := gorm.Open(mysql.New(config), &gorm.Config{
		Logger:         logger,
		NamingStrategy: schema.NamingStrategy{SingularTable: true},
		PrepareStmt:    true,
	})
	if err != nil {
		return nil, fmt.Errorf("gorm.Open failed: %w", err)
	}

	// 获取底层 SQL 连接
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}

	sqlDB.SetMaxOpenConns(cfg.MaxOpenConn)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConn)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	if err = sqlDB.PingContext(ctx); err != nil {
		// Ping 失败时显式关闭无效连接池，与 redis 路径保持一致：
		// 上层 utils.WithBackoff 会重试，不关闭就会累积泄漏 sqlDB 实例。
		if cErr := sqlDB.Close(); cErr != nil {
			return nil, errors.Join(fmt.Errorf("database ping failed: %w", err), cErr)
		}
		return nil, fmt.Errorf("database ping failed: %w", err)
	}

	return db, nil
}

type MySQLConfig struct {
	MySQLConfig *config.MySQLConfig
	LogConfig   *config.LogConfig
	RetryConfig *config.RetryConfig
}

// MySql 初始化数据库
func initMySQL(cfg *MySQLConfig) (db *gorm.DB) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	password := utils.NewSecureString(os.Getenv("MYSQL_WORK_PASSWORD"))
	defer password.Clear()

	username := os.Getenv("MYSQL_USERNAME") // 账号
	host := os.Getenv("MYSQL_HOST")         // 数据库地址
	port := os.Getenv("MYSQL_PORT")         // 数据库端口
	dbName := os.Getenv("MYSQL_DB_NAME")    // 数据库名
	// dsn := "用户名:密码@tcp(地址:端口)/数据库名"
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local", username, password.Get(), host, port, dbName)

	// 配置 Gorm 连接到 MySQL
	mysqlConfig := mysql.Config{
		DSN:                       dsn,   // DSN
		DefaultStringSize:         255,   // string 类型字段的默认长度
		SkipInitializeWithVersion: false, // 根据当前 MySQL 版本自动配置
	}

	// 创建全量 SQL 日志记录器
	fullLogger := logger.New(
		log.New(GetLogWriter(cfg.LogConfig, cfg.LogConfig.MySQLFullLog), "\r\n", log.LstdFlags),
		logger.Config{
			SlowThreshold:             0, // 记录所有SQL，无论快慢
			LogLevel:                  logger.Info,
			IgnoreRecordNotFoundError: true,
			Colorful:                  false,
			ParameterizedQueries:      true,
		},
	)

	// 创建慢 SQL 日志记录器
	slowLogger := logger.New(
		log.New(GetLogWriter(cfg.LogConfig, cfg.LogConfig.MySQLSlowLog), "\r\n", log.LstdFlags),
		logger.Config{
			SlowThreshold:             50 * time.Millisecond, // 只记录超过50ms的SQL
			LogLevel:                  logger.Warn,
			IgnoreRecordNotFoundError: true,
			Colorful:                  false,
			ParameterizedQueries:      true,
		},
	)

	// 组合日志记录器
	compositeLogger := &loggers.CompositeLogger{
		FullLogger: fullLogger,
		SlowLogger: slowLogger,
	}

	// 连接 MySQL
	var err error
	if err = utils.WithBackoff(ctx, cfg.RetryConfig, func() error {
		db, err = ConnectMySQLClient(ctx, mysqlConfig, compositeLogger, cfg.MySQLConfig)
		return err
	}); err != nil {
		panic("MySQL connection failed: " + err.Error())
	}

	return db
}

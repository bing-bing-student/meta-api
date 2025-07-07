package bootstrap

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"

	"meta-api/config"
	"meta-api/internal/app/model"
	"meta-api/internal/common/loggers"
	"meta-api/internal/common/utils"
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

	// 获取底层SQL连接
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}

	sqlDB.SetMaxOpenConns(cfg.MaxOpenConn)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConn)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	if err = sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close() // 关闭无效连接
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

	password := utils.NewSecureString(os.Getenv("MYSQL_PASSWORD"))
	defer password.Clear()

	username := os.Getenv("MYSQL_USERNAME") // 账号
	host := os.Getenv("MYSQL_HOST")         // 数据库地址
	port := os.Getenv("MYSQL_PORT")         // 数据库端口
	dbName := os.Getenv("MYSQL_DB_NAME")    // 数据库名
	// dsn := "用户名:密码@tcp(地址:端口)/数据库名"
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local", username, password.Get(), host, port, dbName)

	// 配置Gorm连接到MySQL
	mysqlConfig := mysql.Config{
		DSN:                       dsn,   // DSN
		DefaultStringSize:         255,   // string 类型字段的默认长度
		SkipInitializeWithVersion: false, // 根据当前 MySQL 版本自动配置
	}

	// 创建全量SQL日志记录器
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

	// 创建慢SQL日志记录器
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

	// 连接MySQL
	var err error
	if err = utils.WithBackoff(ctx, cfg.RetryConfig, func() error {
		db, err = ConnectMySQLClient(ctx, mysqlConfig, compositeLogger, cfg.MySQLConfig)
		return err
	}); err != nil {
		panic("MySQL connection failed: " + err.Error())
	}

	models := []any{
		&model.Article{},
		&model.Tag{},
		&model.Link{},
		&model.Admin{},
	}

	// 自动生成对应的数据库表(表级别的字符排序默认使用utf8mb4_general_ci)
	if err = db.Set("gorm:table_options", "ENGINE=InnoDB CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci").
		AutoMigrate(models...); err != nil {
		panic("failed to auto migrate tables: " + err.Error())
	}

	// 根据特定的业务场景修改特定字段的字符排序规则
	db.Exec("ALTER TABLE tag MODIFY COLUMN name VARCHAR(30) COLLATE utf8mb4_bin NOT NULL;")
	db.Exec("ALTER TABLE link MODIFY COLUMN name VARCHAR(20) COLLATE utf8mb4_bin NOT NULL;")
	db.Exec("ALTER TABLE article MODIFY COLUMN title VARCHAR(30) COLLATE utf8mb4_bin NOT NULL;")

	return db
}

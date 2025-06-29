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
	"meta-api/internal/app/model/admin"
	"meta-api/internal/app/model/article"
	"meta-api/internal/app/model/link"
	"meta-api/internal/app/model/tag"
	"meta-api/internal/common/utils"
)

type CompositeLogger struct {
	fullLogger logger.Interface
	slowLogger logger.Interface
}

func (c *CompositeLogger) LogMode(level logger.LogLevel) logger.Interface {
	c.fullLogger.LogMode(level)
	c.slowLogger.LogMode(level)
	return c
}

func (c *CompositeLogger) Info(ctx context.Context, msg string, data ...interface{}) {
	c.fullLogger.Info(ctx, msg, data...)
}

func (c *CompositeLogger) Warn(ctx context.Context, msg string, data ...interface{}) {
	c.fullLogger.Warn(ctx, msg, data...)
	c.slowLogger.Warn(ctx, msg, data...) // 慢日志也记录Warn
}

func (c *CompositeLogger) Error(ctx context.Context, msg string, data ...interface{}) {
	c.fullLogger.Error(ctx, msg, data...)
	c.slowLogger.Error(ctx, msg, data...)
}

func (c *CompositeLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	// 全量日志记录所有SQL
	c.fullLogger.Trace(ctx, begin, fc, err)

	// 慢日志只记录超过阈值的SQL
	c.slowLogger.Trace(ctx, begin, fc, err)
}

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

// MySql 初始化数据库
func initMySQL(cfg *config.MySQLConfig, logCfg *config.LogConfig) *gorm.DB {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	username := os.Getenv("MYSQL_USERNAME") // 账号
	password := os.Getenv("MYSQL_PASSWORD") // 密码
	host := os.Getenv("MYSQL_HOST")         // 数据库地址，可以是IP或者域名
	port := os.Getenv("MYSQL_PORT")         // 数据库端口
	dbName := os.Getenv("MYSQL_DB_NAME")    // 数据库名
	// dsn := "用户名:密码@tcp(地址:端口)/数据库名"
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local", username, password, host, port, dbName)

	// 配置Gorm连接到MySQL
	mysqlConfig := mysql.Config{
		DSN:                       dsn,   // DSN
		DefaultStringSize:         255,   // string 类型字段的默认长度
		SkipInitializeWithVersion: false, // 根据当前 MySQL 版本自动配置
	}

	// 创建全量SQL日志记录器
	fullLogger := logger.New(
		log.New(GetLogWriter(logCfg, logCfg.MySQLFullLog), "\r\n", log.LstdFlags),
		logger.Config{
			SlowThreshold:             0,           // 记录所有SQL，无论快慢
			LogLevel:                  logger.Info, // 记录所有级别
			IgnoreRecordNotFoundError: true,
			Colorful:                  false,
			ParameterizedQueries:      true,
		},
	)

	// 创建慢SQL日志记录器
	slowLogger := logger.New(
		log.New(GetLogWriter(logCfg, logCfg.MySQLSlowLog), "\r\n", log.LstdFlags),
		logger.Config{
			SlowThreshold:             10 * time.Millisecond, // 只记录超过10ms的SQL
			LogLevel:                  logger.Warn,           // 只记录慢查询
			IgnoreRecordNotFoundError: true,
			Colorful:                  false,
			ParameterizedQueries:      true,
		},
	)

	// 组合日志记录器
	compositeLogger := &CompositeLogger{
		fullLogger: fullLogger,
		slowLogger: slowLogger,
	}

	// 连接MySQL
	var db *gorm.DB
	var err error
	if err = utils.WithBackoff(ctx, func() error {
		db, err = ConnectMySQLClient(ctx, mysqlConfig, compositeLogger, cfg)
		return err
	}); err != nil {
		panic("MySQL connection failed: " + err.Error())
	}

	// 自动生成对应的数据库表(表级别的字符排序默认使用utf8mb4_general_ci)
	if err = db.Set("gorm:table_options", "ENGINE=InnoDB CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci").
		AutoMigrate(&article.Article{}, &tag.Tag{}, &link.Link{}, &admin.Admin{}); err != nil {
		panic("failed to auto migrate tables: " + err.Error())
	}

	// 根据特定的业务场景修改特定字段的字符排序规则
	db.Exec("ALTER TABLE tag MODIFY COLUMN name VARCHAR(30) COLLATE utf8mb4_bin NOT NULL;")
	db.Exec("ALTER TABLE link MODIFY COLUMN name VARCHAR(20) COLLATE utf8mb4_bin NOT NULL;")
	db.Exec("ALTER TABLE article MODIFY COLUMN title VARCHAR(30) COLLATE utf8mb4_bin NOT NULL;")
	db.Exec("ALTER TABLE admin MODIFY COLUMN username VARCHAR(20) COLLATE utf8mb4_bin NOT NULL;")
	db.Exec("ALTER TABLE admin MODIFY COLUMN password VARCHAR(100) COLLATE utf8mb4_bin NOT NULL;")

	return db
}

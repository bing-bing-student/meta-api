package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"

	adminModel "meta-api/app/model/admin"
	articleModel "meta-api/app/model/article"
	commentModel "meta-api/app/model/comment"
	linkModel "meta-api/app/model/link"
	siteDynamicModel "meta-api/app/model/sitedynamic"
	tagModel "meta-api/app/model/tag"
	userModel "meta-api/app/model/user"
	"meta-api/common/env"
	"meta-api/common/loggers"
	"meta-api/common/utils"
	"meta-api/config"
)

const (
	defaultMySQLMaxOpenConns  = 40
	defaultMySQLMaxIdleConns  = 10
	defaultMySQLConnLifetime  = 30 * time.Minute
	defaultMySQLConnIdleTime  = 5 * time.Minute
	defaultMySQLSlowThreshold = 100 * time.Millisecond
)

// ConnectMySQLClient 初始化MySQL客户端
func ConnectMySQLClient(ctx context.Context, config mysql.Config, logger logger.Interface, cfg *config.MySQLConfig) (*gorm.DB, error) {
	if cfg == nil {
		return nil, fmt.Errorf("mysql connection config is nil")
	}

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

	applyMySQLPoolConfig(sqlDB, cfg)

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

type mysqlEnv struct {
	username string
	password string
	host     string
	port     string
	dbName   string
}

// MySql 初始化数据库
func initMySQL(cfg *MySQLConfig) (*gorm.DB, error) {
	if err := validateMySQLConfig(cfg); err != nil {
		return nil, err
	}

	env, err := mysqlEnvFromEnv()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	password := utils.NewSecureString(env.password)
	defer password.Clear()

	// dsn := "用户名:密码@tcp(地址:端口)/数据库名"
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		env.username, password.Get(), env.host, env.port, env.dbName)

	// 配置 Gorm 连接到 MySQL
	mysqlConfig := mysql.Config{
		DSN:                       dsn,   // DSN
		DefaultStringSize:         255,   // string 类型字段的默认长度
		SkipInitializeWithVersion: false, // 根据当前 MySQL 版本自动配置
	}

	mysqlLogger := newMySQLLogger(cfg)

	// 连接 MySQL
	var db *gorm.DB
	if err = utils.WithBackoff(ctx, cfg.RetryConfig, func() error {
		db, err = ConnectMySQLClient(ctx, mysqlConfig, mysqlLogger, cfg.MySQLConfig)
		return err
	}); err != nil {
		return nil, fmt.Errorf("mysql connection failed: %w", err)
	}

	return db, nil
}

func validateMySQLConfig(cfg *MySQLConfig) error {
	if cfg == nil {
		return fmt.Errorf("mysql config is nil")
	}
	if cfg.MySQLConfig == nil {
		return fmt.Errorf("mysql connection config is nil")
	}
	if cfg.LogConfig == nil {
		return fmt.Errorf("mysql log config is nil")
	}
	if cfg.RetryConfig == nil {
		return fmt.Errorf("mysql retry config is nil")
	}
	return nil
}

type sqlConnectionPool interface {
	SetMaxOpenConns(int)
	SetMaxIdleConns(int)
	SetConnMaxLifetime(time.Duration)
	SetConnMaxIdleTime(time.Duration)
}

func applyMySQLPoolConfig(sqlDB sqlConnectionPool, cfg *config.MySQLConfig) {
	maxOpenConns := cfg.MaxOpenConn
	if maxOpenConns <= 0 {
		maxOpenConns = defaultMySQLMaxOpenConns
	}

	maxIdleConns := cfg.MaxIdleConn
	if maxIdleConns <= 0 {
		maxIdleConns = defaultMySQLMaxIdleConns
	}
	if maxIdleConns > maxOpenConns {
		maxIdleConns = maxOpenConns
	}

	connMaxLifetime := cfg.ConnMaxLifetime
	if connMaxLifetime <= 0 {
		connMaxLifetime = defaultMySQLConnLifetime
	}

	connMaxIdleTime := cfg.ConnMaxIdleTime
	if connMaxIdleTime <= 0 {
		connMaxIdleTime = defaultMySQLConnIdleTime
	}

	sqlDB.SetMaxOpenConns(maxOpenConns)
	sqlDB.SetMaxIdleConns(maxIdleConns)
	sqlDB.SetConnMaxLifetime(connMaxLifetime)
	sqlDB.SetConnMaxIdleTime(connMaxIdleTime)
}

func newMySQLLogger(cfg *MySQLConfig) logger.Interface {
	slowThreshold := cfg.MySQLConfig.SlowThreshold
	if slowThreshold <= 0 {
		slowThreshold = defaultMySQLSlowThreshold
	}

	slowLogger := logger.New(
		log.New(GetLogWriter(cfg.LogConfig, cfg.LogConfig.MySQLSlowLog), "\r\n", log.LstdFlags),
		logger.Config{
			SlowThreshold:             slowThreshold,
			LogLevel:                  logger.Warn,
			IgnoreRecordNotFoundError: true,
			Colorful:                  false,
			ParameterizedQueries:      true,
		},
	)

	if !cfg.MySQLConfig.LogFullSQL {
		return slowLogger
	}

	fullLogger := logger.New(
		log.New(GetLogWriter(cfg.LogConfig, cfg.LogConfig.MySQLFullLog), "\r\n", log.LstdFlags),
		logger.Config{
			SlowThreshold:             0,
			LogLevel:                  logger.Info,
			IgnoreRecordNotFoundError: true,
			Colorful:                  false,
			ParameterizedQueries:      true,
		},
	)

	return &loggers.CompositeLogger{
		FullLogger: fullLogger,
		SlowLogger: slowLogger,
	}
}

func mysqlEnvFromEnv() (*mysqlEnv, error) {
	values, err := requiredEnvValues(
		env.MySQLUsername,
		env.MySQLHost,
		env.MySQLPort,
		env.MySQLDBName,
	)
	if err != nil {
		return nil, err
	}
	password, err := utils.RequiredEnvOrFile(env.MySQLWorkPassword)
	if err != nil {
		return nil, err
	}
	return &mysqlEnv{
		username: strings.TrimSpace(values[env.MySQLUsername]),
		password: password,
		host:     strings.TrimSpace(values[env.MySQLHost]),
		port:     strings.TrimSpace(values[env.MySQLPort]),
		dbName:   strings.TrimSpace(values[env.MySQLDBName]),
	}, nil
}

func autoMigrateMySQL(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("mysql db is nil")
	}
	if err := db.AutoMigrate(
		&adminModel.Admin{},
		&tagModel.Tag{},
		&articleModel.Article{},
		&articleModel.ArticleImage{},
		&articleModel.ArticleImageReference{},
		&linkModel.Link{},
		&siteDynamicModel.SiteDynamic{},
		&userModel.User{},
		&commentModel.Comment{},
		&commentModel.CommentReport{},
		&commentModel.CommentModerationAudit{},
		&commentModel.CommentModerationFeedback{},
	); err != nil {
		return fmt.Errorf("auto migrate mysql tables: %w", err)
	}
	return nil
}

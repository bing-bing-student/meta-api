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

func applyMySQLPoolConfig(sqlDB interface {
	SetMaxOpenConns(int)
	SetMaxIdleConns(int)
	SetConnMaxLifetime(time.Duration)
	SetConnMaxIdleTime(time.Duration)
}, cfg *config.MySQLConfig) {
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
	if err := renameArticleImageNameColumn(db); err != nil {
		return err
	}
	if err := db.AutoMigrate(
		&adminModel.Admin{},
		&tagModel.Tag{},
		&articleModel.Article{},
		&articleModel.ArticleImage{},
		&articleModel.ArticleImageReference{},
		&linkModel.Link{},
		&userModel.User{},
		&commentModel.Comment{},
		&commentModel.CommentReport{},
	); err != nil {
		return fmt.Errorf("auto migrate mysql tables: %w", err)
	}
	if err := dropArticleTitleUniqueIndexes(db); err != nil {
		return err
	}
	if err := relaxArticleDraftColumns(db); err != nil {
		return err
	}
	return nil
}

func renameArticleImageNameColumn(db *gorm.DB) error {
	hasFileName, err := mysqlColumnExists(db, "article_image", "file_name")
	if err != nil {
		return err
	}
	if !hasFileName {
		return nil
	}

	hasImageName, err := mysqlColumnExists(db, "article_image", "image_name")
	if err != nil {
		return err
	}
	if hasImageName {
		if err := db.Exec("UPDATE `article_image` SET `image_name` = `file_name` WHERE (`image_name` IS NULL OR `image_name` = '') AND `file_name` <> ''").Error; err != nil {
			return fmt.Errorf("backfill article_image image_name column: %w", err)
		}
		if err := db.Exec("ALTER TABLE `article_image` DROP COLUMN `file_name`").Error; err != nil {
			return fmt.Errorf("drop article_image file_name column: %w", err)
		}
		return nil
	}

	if err := db.Exec("ALTER TABLE `article_image` CHANGE COLUMN `file_name` `image_name` varchar(255) NOT NULL").Error; err != nil {
		return fmt.Errorf("rename article_image file_name column: %w", err)
	}
	return nil
}

func dropArticleTitleUniqueIndexes(db *gorm.DB) error {
	type indexRow struct {
		IndexName string `gorm:"column:INDEX_NAME"`
	}

	rows := make([]indexRow, 0)
	if err := db.Raw(`
SELECT INDEX_NAME
FROM INFORMATION_SCHEMA.STATISTICS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'article'
  AND COLUMN_NAME = 'title'
  AND NON_UNIQUE = 0
  AND INDEX_NAME <> 'PRIMARY'
`).Scan(&rows).Error; err != nil {
		return fmt.Errorf("query article title unique indexes: %w", err)
	}

	for _, row := range rows {
		indexName := strings.TrimSpace(row.IndexName)
		if indexName == "" {
			continue
		}
		quoted := "`" + strings.ReplaceAll(indexName, "`", "``") + "`"
		if err := db.Exec("ALTER TABLE `article` DROP INDEX " + quoted).Error; err != nil {
			return fmt.Errorf("drop article title unique index %s: %w", indexName, err)
		}
	}
	return nil
}

func relaxArticleDraftColumns(db *gorm.DB) error {
	type columnRow struct {
		IsNullable string `gorm:"column:IS_NULLABLE"`
	}

	column := &columnRow{}
	if err := db.Raw(`
SELECT IS_NULLABLE
FROM INFORMATION_SCHEMA.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'article'
  AND COLUMN_NAME = 'tag_id'
`).Scan(column).Error; err != nil {
		return fmt.Errorf("query article tag_id nullability: %w", err)
	}
	if strings.EqualFold(strings.TrimSpace(column.IsNullable), "YES") {
		return nil
	}

	if err := db.Exec("ALTER TABLE `article` MODIFY COLUMN `tag_id` bigint unsigned NULL").Error; err != nil {
		return fmt.Errorf("relax article tag_id nullability: %w", err)
	}
	return nil
}

func mysqlColumnExists(db *gorm.DB, tableName string, columnName string) (bool, error) {
	var count int64
	if err := db.Raw(`
SELECT COUNT(*)
FROM INFORMATION_SCHEMA.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = ?
  AND COLUMN_NAME = ?
`, tableName, columnName).Scan(&count).Error; err != nil {
		return false, fmt.Errorf("query mysql column %s.%s: %w", tableName, columnName, err)
	}
	return count > 0, nil
}

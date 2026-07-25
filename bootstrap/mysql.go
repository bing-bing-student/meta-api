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

	"meta-api/app/model/admin"
	"meta-api/app/model/article"
	"meta-api/app/model/comment"
	"meta-api/app/model/link"
	"meta-api/app/model/tag"
	"meta-api/app/model/user"
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

	renameLegacyUserAccountTable(db)
	renameLegacyUserIndexes(db)
	prepareUserHandleMigration(db)

	models := []any{
		&article.Article{},
		&tag.Tag{},
		&link.Link{},
		&admin.Admin{},
		&user.User{},
		&comment.Comment{},
	}

	// 自动生成对应的数据库表(表级别的字符排序默认使用utf8mb4_general_ci)
	if err = db.Set("gorm:table_options", "ENGINE=InnoDB CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci").
		AutoMigrate(models...); err != nil {
		panic("failed to auto migrate tables: " + err.Error())
	}
	dropLegacyCommentColumns(db)

	// 根据特定的业务场景修改特定字段的字符排序规则
	db.Exec("ALTER TABLE tag MODIFY COLUMN name VARCHAR(30) COLLATE utf8mb4_bin NOT NULL;")
	db.Exec("ALTER TABLE link MODIFY COLUMN name VARCHAR(20) COLLATE utf8mb4_bin NOT NULL;")
	db.Exec("ALTER TABLE article MODIFY COLUMN title VARCHAR(30) COLLATE utf8mb4_bin NOT NULL;")

	return db
}

func dropLegacyCommentColumns(db *gorm.DB) {
	for _, column := range []string{"author_email", "author_website", "user_agent"} {
		if db.Migrator().HasColumn(&comment.Comment{}, column) {
			_ = db.Migrator().DropColumn(&comment.Comment{}, column)
		}
	}
}

func renameLegacyUserAccountTable(db *gorm.DB) {
	if !db.Migrator().HasTable("user_account") || db.Migrator().HasTable("user") {
		return
	}
	if err := db.Migrator().RenameTable("user_account", "user"); err != nil {
		panic("failed to rename user_account table: " + err.Error())
	}
}

func renameLegacyUserIndexes(db *gorm.DB) {
	const (
		legacyProviderUIDIndex = "idx_user_account_provider_uid"
		providerUIDIndex       = "idx_user_provider_uid"
	)

	if !db.Migrator().HasIndex(&user.User{}, legacyProviderUIDIndex) ||
		db.Migrator().HasIndex(&user.User{}, providerUIDIndex) {
		return
	}
	if err := db.Migrator().RenameIndex(&user.User{}, legacyProviderUIDIndex, providerUIDIndex); err != nil {
		panic("failed to rename user provider index: " + err.Error())
	}
}

func prepareUserHandleMigration(db *gorm.DB) {
	if !db.Migrator().HasTable(&user.User{}) {
		return
	}
	if !db.Migrator().HasColumn(&user.User{}, "handle") {
		if err := db.Exec("ALTER TABLE `user` ADD COLUMN `handle` VARCHAR(32) NULL AFTER `display_name`;").Error; err != nil {
			panic("failed to add user handle column: " + err.Error())
		}
	}

	if err := db.Exec("UPDATE `user` SET `handle` = CONCAT('tmp-', `id`) WHERE `handle` IS NULL OR `handle` = '';").Error; err != nil {
		panic("failed to prepare user handle column: " + err.Error())
	}
	if err := db.Exec(`
		UPDATE ` + "`user`" + ` u
		JOIN (
			SELECT ordered.id,
				IF(ordered.seq < 100000, LPAD(CAST(ordered.seq AS CHAR), 5, '0'), CAST(ordered.seq AS CHAR)) AS next_handle
			FROM (
				SELECT source.id, @next_user_handle := @next_user_handle + 1 AS seq
				FROM (
					SELECT id
					FROM ` + "`user`" + `
					WHERE ` + "`handle`" + ` NOT REGEXP '^[0-9]+$' OR CAST(` + "`handle`" + ` AS UNSIGNED) = 0
					ORDER BY ` + "`create_time`" + `, ` + "`id`" + `
				) source
				JOIN (
					SELECT @next_user_handle := (
						SELECT COALESCE(MAX(CAST(` + "`handle`" + ` AS UNSIGNED)), 0)
						FROM ` + "`user`" + `
						WHERE ` + "`handle`" + ` REGEXP '^[0-9]+$' AND CAST(` + "`handle`" + ` AS UNSIGNED) > 0
					)
				) vars
			) ordered
		) mapped ON mapped.id = u.id
		SET u.handle = mapped.next_handle;
	`).Error; err != nil {
		panic("failed to backfill numeric user handles: " + err.Error())
	}
	if err := db.Exec("ALTER TABLE `user` MODIFY COLUMN `handle` VARCHAR(32) NOT NULL;").Error; err != nil {
		panic("failed to require user handle column: " + err.Error())
	}
	if !db.Migrator().HasIndex(&user.User{}, "idx_user_handle") {
		if err := db.Exec("CREATE UNIQUE INDEX `idx_user_handle` ON `user` (`handle`);").Error; err != nil {
			panic("failed to create user handle index: " + err.Error())
		}
	}
}

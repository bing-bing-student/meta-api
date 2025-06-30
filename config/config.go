package config

import "time"

// LogConfig 配置日志的结构体
type LogConfig struct {
	MySQLFullLog string `mapstructure:"mysql_full_log"`
	MySQLSlowLog string `mapstructure:"mysql_slow_log"`
	HTTPFullLog  string `mapstructure:"http_full_log"`
	HTTPErrLog   string `mapstructure:"http_err_log"`
	MaxSize      int    `mapstructure:"max_size"`
	MaxAge       int    `mapstructure:"max_age"`
	MaxBackups   int    `mapstructure:"max_backups"`
	Compress     bool   `mapstructure:"compress"`
}

// RetryConfig 定义重试配置文件结构体
type RetryConfig struct {
	MaxRetries   int           `mapstructure:"max_retries"`
	InitialDelay time.Duration `mapstructure:"initial_delay"`
	MaxDelay     time.Duration `mapstructure:"max_delay"`
	JitterFactor float64       `mapstructure:"jitter_factor"`
}

// MySQLConfig 定义 mysql 配置文件结构体
type MySQLConfig struct {
	MaxOpenConn     int           `mapstructure:"max_open_conn"`
	MaxIdleConn     int           `mapstructure:"max_idle_conn"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
}

// RedisConfig 定义 redis 配置文件结构体
type RedisConfig struct {
	DB int `mapstructure:"db"`
}

// AdminInfoConfig 定义管理员配置文件结构体
type AdminInfoConfig struct {
	Issuer      string `mapstructure:"issuer"`
	AccountName string `mapstructure:"account_name"`
}

// Config 定义项目配置文件结构体
type Config struct {
	LogConfig       *LogConfig       `mapstructure:"log"`
	RetryConfig     *RetryConfig     `mapstructure:"retry"`
	MySQLConfig     *MySQLConfig     `mapstructure:"mysql"`
	RedisConfig     *RedisConfig     `mapstructure:"redis"`
	AdminInfoConfig *AdminInfoConfig `mapstructure:"admin_info"`
}

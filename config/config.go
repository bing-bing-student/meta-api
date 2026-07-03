package config

import (
	"sync"
	"time"
)

// LogConfig 配置日志的结构体
type LogConfig struct {
	MySQLFullLog string `mapstructure:"mysql_full_log"`
	MySQLSlowLog string `mapstructure:"mysql_slow_log"`
	HTTPInfoLog  string `mapstructure:"http_info_log"`
	HTTPWarnLog  string `mapstructure:"http_warn_log"`
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

// GuardConfig 风控守卫引擎配置。
type GuardConfig struct {
	BuildHashes       []string `mapstructure:"build_hashes"`
	SkipHMACWhenEmpty bool     `mapstructure:"skip_hmac_when_empty"`
}

// RateLimitWindowConfig 描述一条限流窗口规则。
type RateLimitWindowConfig struct {
	Limit         int64 `mapstructure:"limit"`
	WindowSeconds int64 `mapstructure:"window_seconds"`
}

// AccountLoginRateLimitConfig 描述账号密码登录限流策略。
type AccountLoginRateLimitConfig struct {
	IP                   RateLimitWindowConfig `mapstructure:"ip"`
	Username             RateLimitWindowConfig `mapstructure:"username"`
	UsernameIP           RateLimitWindowConfig `mapstructure:"username_ip"`
	FailureThreshold     int64                 `mapstructure:"failure_threshold"`
	FailureWindowSeconds int64                 `mapstructure:"failure_window_seconds"`
	LockLevelTTLSeconds  int64                 `mapstructure:"lock_level_ttl_seconds"`
	LockDurationsSeconds []int64               `mapstructure:"lock_durations_seconds"`
}

// DynamicCodeRateLimitConfig 描述 TOTP 绑定/验证限流策略。
type DynamicCodeRateLimitConfig struct {
	IP                   RateLimitWindowConfig `mapstructure:"ip"`
	Challenge            RateLimitWindowConfig `mapstructure:"challenge"`
	FailureThreshold     int64                 `mapstructure:"failure_threshold"`
	FailureWindowSeconds int64                 `mapstructure:"failure_window_seconds"`
}

// AdminLoginRateLimitConfig 描述后台登录链路限流策略。
type AdminLoginRateLimitConfig struct {
	Disabled          bool                        `mapstructure:"disabled"`
	AccountLogin      AccountLoginRateLimitConfig `mapstructure:"account_login"`
	BindDynamicCode   DynamicCodeRateLimitConfig  `mapstructure:"bind_dynamic_code"`
	VerifyDynamicCode DynamicCodeRateLimitConfig  `mapstructure:"verify_dynamic_code"`
}

// RateLimitConfig 描述后端应用级限流配置。
type RateLimitConfig struct {
	AdminLogin AdminLoginRateLimitConfig `mapstructure:"admin_login"`
}

// Config 定义项目配置文件结构体
type Config struct {
	mu sync.RWMutex

	LogConfig       *LogConfig       `mapstructure:"log"`
	RetryConfig     *RetryConfig     `mapstructure:"retry"`
	MySQLConfig     *MySQLConfig     `mapstructure:"mysql"`
	RedisConfig     *RedisConfig     `mapstructure:"redis"`
	AdminInfoConfig *AdminInfoConfig `mapstructure:"admin_info"`
	GuardConfig     *GuardConfig     `mapstructure:"guard"`
	RateLimitConfig *RateLimitConfig `mapstructure:"rate_limit"`
}

// Replace 原子替换可热更新配置。调用方应先反序列化到临时 Config，成功后再替换。
func (c *Config) Replace(next *Config) {
	if c == nil || next == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	c.LogConfig = next.LogConfig
	c.RetryConfig = next.RetryConfig
	c.MySQLConfig = next.MySQLConfig
	c.RedisConfig = next.RedisConfig
	c.AdminInfoConfig = next.AdminInfoConfig
	c.GuardConfig = next.GuardConfig
	c.RateLimitConfig = next.RateLimitConfig
}

// AdminInfoSnapshot 返回管理员信息配置快照。
func (c *Config) AdminInfoSnapshot() AdminInfoConfig {
	if c == nil {
		return AdminInfoConfig{}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.AdminInfoConfig == nil {
		return AdminInfoConfig{}
	}
	return *c.AdminInfoConfig
}

// RateLimitSnapshot 返回限流配置快照。
func (c *Config) RateLimitSnapshot() RateLimitConfig {
	if c == nil {
		return RateLimitConfig{}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.RateLimitConfig == nil {
		return RateLimitConfig{}
	}
	snapshot := *c.RateLimitConfig
	snapshot.AdminLogin.AccountLogin.LockDurationsSeconds = cloneInt64Slice(
		snapshot.AdminLogin.AccountLogin.LockDurationsSeconds,
	)
	return snapshot
}

// cloneInt64Slice 复制 int64 切片，避免快照共享底层数组。
func cloneInt64Slice(src []int64) []int64 {
	if len(src) == 0 {
		return nil
	}
	dst := make([]int64, len(src))
	copy(dst, src)
	return dst
}

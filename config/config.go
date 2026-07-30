package config

import (
	"strings"
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

// OAuthProviderConfig 定义单个 OAuth Provider 的非敏感配置。
type OAuthProviderConfig struct {
	ClientID    string `mapstructure:"client_id"`
	RedirectURI string `mapstructure:"redirect_uri"`
}

// OAuthConfig 定义前台用户登录 OAuth 配置。
type OAuthConfig struct {
	GitHub OAuthProviderConfig `mapstructure:"github"`
	Google OAuthProviderConfig `mapstructure:"google"`
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

// CommentSubmitRateLimitConfig 描述前台评论提交限流策略。
type CommentSubmitRateLimitConfig struct {
	Disabled    bool                  `mapstructure:"disabled"`
	IP          RateLimitWindowConfig `mapstructure:"ip"`
	User        RateLimitWindowConfig `mapstructure:"user"`
	UserArticle RateLimitWindowConfig `mapstructure:"user_article"`
}

// CommentReportRateLimitConfig 描述前台评论举报限流策略。
type CommentReportRateLimitConfig struct {
	Disabled  bool                  `mapstructure:"disabled"`
	IP        RateLimitWindowConfig `mapstructure:"ip"`
	User      RateLimitWindowConfig `mapstructure:"user"`
	IPComment RateLimitWindowConfig `mapstructure:"ip_comment"`
}

// CommentModerationScoreConfig 描述评论审核评分决策阈值。
type CommentModerationScoreConfig struct {
	Pending int `mapstructure:"pending"`
	Reject  int `mapstructure:"reject"`
}

// CommentModerationLexiconConfig 描述敏感词识别层配置。
type CommentModerationLexiconConfig struct {
	Provider           string                             `mapstructure:"provider"`
	UseBuiltin         bool                               `mapstructure:"use_builtin"`
	StrictBuiltinMatch bool                               `mapstructure:"strict_builtin_match"`
	CustomWords        CommentModerationCustomWordsConfig `mapstructure:"custom_words"`
}

// CommentModerationCustomWordsConfig 描述后续微调用的自定义词库。
type CommentModerationCustomWordsConfig struct {
	Block  map[string][]string `mapstructure:"block"`
	Review map[string][]string `mapstructure:"review"`
}

// CommentModerationLevelRuleConfig 描述只有处置等级的规则配置。
type CommentModerationLevelRuleConfig struct {
	Level string `mapstructure:"level"`
}

// CommentModerationBehaviorThresholdConfig 描述评论审核行为阈值规则。
type CommentModerationBehaviorThresholdConfig struct {
	WindowSeconds   int64 `mapstructure:"window_seconds"`
	ReviewThreshold int64 `mapstructure:"review_threshold"`
	BlockThreshold  int64 `mapstructure:"block_threshold"`
}

// CommentModerationBehaviorRulesConfig 描述评论审核行为风险规则。
type CommentModerationBehaviorRulesConfig struct {
	UserFrequency    CommentModerationBehaviorThresholdConfig `mapstructure:"user_frequency"`
	IPFrequency      CommentModerationBehaviorThresholdConfig `mapstructure:"ip_frequency"`
	DuplicateContent CommentModerationBehaviorThresholdConfig `mapstructure:"duplicate_content"`
}

// CommentModerationCategoryDecisionConfig 描述分类级处置覆盖。
type CommentModerationCategoryDecisionConfig struct {
	Level string `mapstructure:"level"`
}

// CommentModerationDecisionConfig 描述评论审核最终决策策略。
type CommentModerationDecisionConfig struct {
	DefaultOnError    string                                             `mapstructure:"default_on_error"`
	Score             CommentModerationScoreConfig                       `mapstructure:"score"`
	CategoryOverrides map[string]CommentModerationCategoryDecisionConfig `mapstructure:"category_overrides"`
}

// CommentModerationContextRuleConfig 描述“主体词 + 行为词”组合规则。
type CommentModerationContextRuleConfig struct {
	ID         string   `mapstructure:"id"`
	Name       string   `mapstructure:"name"`
	Category   string   `mapstructure:"category"`
	Level      string   `mapstructure:"level"`
	Subjects   []string `mapstructure:"subjects"`
	Predicates []string `mapstructure:"predicates"`
}

// CommentModerationConfig 描述前台评论审核策略。
type CommentModerationConfig struct {
	Disabled        bool                                        `mapstructure:"disabled"`
	ReportThreshold int64                                       `mapstructure:"report_threshold"`
	Lexicon         CommentModerationLexiconConfig              `mapstructure:"lexicon"`
	StructureRules  map[string]CommentModerationLevelRuleConfig `mapstructure:"structure_rules"`
	ContextRules    []CommentModerationContextRuleConfig        `mapstructure:"context_rules"`
	BehaviorRules   CommentModerationBehaviorRulesConfig        `mapstructure:"behavior_rules"`
	Decision        CommentModerationDecisionConfig             `mapstructure:"decision"`
}

// RateLimitConfig 描述后端应用级限流配置。
type RateLimitConfig struct {
	AdminLogin    AdminLoginRateLimitConfig    `mapstructure:"admin_login"`
	CommentSubmit CommentSubmitRateLimitConfig `mapstructure:"comment_submit"`
	CommentReport CommentReportRateLimitConfig `mapstructure:"comment_report"`
}

// Config 定义项目配置文件结构体
type Config struct {
	mu sync.RWMutex

	LogConfig               *LogConfig               `mapstructure:"log"`
	RetryConfig             *RetryConfig             `mapstructure:"retry"`
	MySQLConfig             *MySQLConfig             `mapstructure:"mysql"`
	RedisConfig             *RedisConfig             `mapstructure:"redis"`
	OAuthConfig             *OAuthConfig             `mapstructure:"oauth"`
	AdminInfoConfig         *AdminInfoConfig         `mapstructure:"admin_info"`
	GuardConfig             *GuardConfig             `mapstructure:"guard"`
	RateLimitConfig         *RateLimitConfig         `mapstructure:"rate_limit"`
	CommentModerationConfig *CommentModerationConfig `mapstructure:"comment_moderation"`
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
	c.OAuthConfig = next.OAuthConfig
	c.AdminInfoConfig = next.AdminInfoConfig
	c.GuardConfig = next.GuardConfig
	c.RateLimitConfig = next.RateLimitConfig
	c.CommentModerationConfig = next.CommentModerationConfig
}

// OAuthProviderSnapshot 返回指定 OAuth Provider 的配置快照。
func (c *Config) OAuthProviderSnapshot(provider string) OAuthProviderConfig {
	if c == nil {
		return OAuthProviderConfig{}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.OAuthConfig == nil {
		return OAuthProviderConfig{}
	}
	switch strings.ToLower(provider) {
	case "github":
		return c.OAuthConfig.GitHub
	case "google":
		return c.OAuthConfig.Google
	default:
		return OAuthProviderConfig{}
	}
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

// CommentModerationSnapshot 返回评论审核配置快照。
func (c *Config) CommentModerationSnapshot() CommentModerationConfig {
	if c == nil {
		return CommentModerationConfig{}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.CommentModerationConfig == nil {
		return CommentModerationConfig{}
	}
	snapshot := *c.CommentModerationConfig
	snapshot.Lexicon.CustomWords = cloneCommentModerationCustomWordsConfig(snapshot.Lexicon.CustomWords)
	snapshot.StructureRules = cloneCommentModerationLevelRuleConfigMap(snapshot.StructureRules)
	snapshot.ContextRules = cloneCommentModerationContextRuleConfigSlice(snapshot.ContextRules)
	snapshot.Decision.CategoryOverrides = cloneCommentModerationCategoryDecisionConfigMap(snapshot.Decision.CategoryOverrides)
	return snapshot
}

func cloneCommentModerationCustomWordsConfig(
	src CommentModerationCustomWordsConfig,
) CommentModerationCustomWordsConfig {
	return CommentModerationCustomWordsConfig{
		Block:  cloneStringSliceMap(src.Block),
		Review: cloneStringSliceMap(src.Review),
	}
}

func cloneCommentModerationLevelRuleConfigMap(
	src map[string]CommentModerationLevelRuleConfig,
) map[string]CommentModerationLevelRuleConfig {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]CommentModerationLevelRuleConfig, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func cloneCommentModerationCategoryDecisionConfigMap(
	src map[string]CommentModerationCategoryDecisionConfig,
) map[string]CommentModerationCategoryDecisionConfig {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]CommentModerationCategoryDecisionConfig, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func cloneCommentModerationContextRuleConfigSlice(
	src []CommentModerationContextRuleConfig,
) []CommentModerationContextRuleConfig {
	if len(src) == 0 {
		return nil
	}
	dst := make([]CommentModerationContextRuleConfig, len(src))
	for i, item := range src {
		dst[i] = item
		dst[i].Subjects = cloneStringSlice(item.Subjects)
		dst[i].Predicates = cloneStringSlice(item.Predicates)
	}
	return dst
}

func cloneStringSliceMap(src map[string][]string) map[string][]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string][]string, len(src))
	for key, value := range src {
		dst[key] = cloneStringSlice(value)
	}
	return dst
}

// cloneStringSlice 复制 string 切片，避免快照共享底层数组。
func cloneStringSlice(src []string) []string {
	if len(src) == 0 {
		return nil
	}
	dst := make([]string, len(src))
	copy(dst, src)
	return dst
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

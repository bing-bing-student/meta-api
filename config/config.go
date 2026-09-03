package config

import (
	"maps"
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
	ConnMaxIdleTime time.Duration `mapstructure:"conn_max_idle_time"`
	LogFullSQL      bool          `mapstructure:"log_full_sql"`
	SlowThreshold   time.Duration `mapstructure:"slow_threshold"`
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

// BugFeedbackSMTPConfig 描述 Bug 反馈邮件的非敏感 SMTP 配置。
type BugFeedbackSMTPConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Username string `mapstructure:"username"`
	From     string `mapstructure:"from"`
	FromName string `mapstructure:"from_name"`
}

// BugFeedbackConfig 描述 Bug 反馈功能配置。
type BugFeedbackConfig struct {
	SMTP BugFeedbackSMTPConfig `mapstructure:"smtp"`
}

// ArticleImageCOSConfig 描述文章图片上传到腾讯云 COS 的非敏感配置。
type ArticleImageCOSConfig struct {
	Bucket        string `mapstructure:"bucket"`
	Region        string `mapstructure:"region"`
	Directory     string `mapstructure:"directory"`
	PublicBaseURL string `mapstructure:"public_base_url"`
}

// ArticleImageConfig 描述文章图片资源配置。
type ArticleImageConfig struct {
	COS ArticleImageCOSConfig `mapstructure:"cos"`
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

// BugFeedbackRateLimitConfig 描述 Bug 反馈接口限流策略。
type BugFeedbackRateLimitConfig struct {
	Disabled bool                  `mapstructure:"disabled"`
	IP       RateLimitWindowConfig `mapstructure:"ip"`
}

// CommentModerationLexiconConfig 描述敏感词识别层配置。
type CommentModerationLexiconConfig struct {
	Provider           string                             `mapstructure:"provider"`
	UseBuiltin         bool                               `mapstructure:"use_builtin"`
	StrictBuiltinMatch bool                               `mapstructure:"strict_builtin_match"`
	CustomWords        CommentModerationCustomWordsConfig `mapstructure:"custom_words"`
	Fuzzy              CommentModerationFuzzyConfig       `mapstructure:"fuzzy"`
}

// CommentModerationCustomWordsConfig 描述后续微调用的自定义词库。
type CommentModerationCustomWordsConfig struct {
	Block  map[string][]string `mapstructure:"block"`
	Review map[string][]string `mapstructure:"review"`
}

// CommentModerationFuzzyConfig 描述受限候选集上的模糊词匹配。
type CommentModerationFuzzyConfig struct {
	Disabled       bool                `mapstructure:"disabled"`
	MaxDistance    int                 `mapstructure:"max_distance"`
	MinWordRunes   int                 `mapstructure:"min_word_runes"`
	CandidateWords map[string][]string `mapstructure:"candidate_words"`
}

// CommentModerationLevelRuleConfig 描述只有处置等级的规则配置。
type CommentModerationLevelRuleConfig struct {
	Level string `mapstructure:"level"`
}

// CommentModerationStructurePatternsConfig 描述结构检测层的业务风险词形。
type CommentModerationStructurePatternsConfig struct {
	RiskPhrases           []string `mapstructure:"risk_phrases"`
	RiskPatterns          []string `mapstructure:"risk_patterns"`
	URLPatterns           []string `mapstructure:"url_patterns"`
	ContactPatterns       []string `mapstructure:"contact_patterns"`
	ContactLabels         []string `mapstructure:"contact_labels"`
	NegatedContactMarkers []string `mapstructure:"negated_contact_markers"`
	BenignContactPatterns []string `mapstructure:"benign_contact_patterns"`
	MinNumericRunes       int      `mapstructure:"min_numeric_runes"`
	MinRepeatedRunes      int      `mapstructure:"min_repeated_runes"`
	NumberLikeRatio       float64  `mapstructure:"number_like_ratio"`
	RepeatedRatio         float64  `mapstructure:"repeated_ratio"`
	MinAccountTokenRunes  int      `mapstructure:"min_account_token_runes"`
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
	NearDuplicate    CommentModerationNearDuplicateConfig     `mapstructure:"near_duplicate"`
}

// CommentModerationNearDuplicateConfig 描述 SimHash 近重复检测边界。
type CommentModerationNearDuplicateConfig struct {
	Disabled                   bool  `mapstructure:"disabled"`
	WindowSeconds              int64 `mapstructure:"window_seconds"`
	ReviewThreshold            int64 `mapstructure:"review_threshold"`
	MaxHammingDistance         int   `mapstructure:"max_hamming_distance"`
	MinContentRunes            int   `mapstructure:"min_content_runes"`
	MaxSamples                 int64 `mapstructure:"max_samples"`
	MaxLengthDifferencePercent int   `mapstructure:"max_length_difference_percent"`
}

// CommentModerationDecisionConfig 描述审核异常时的保守处置策略。
// 正常状态与风险分统一由 decision_engine 计算，不再维护平行的人工评分体系。
type CommentModerationDecisionConfig struct {
	DefaultOnError string `mapstructure:"default_on_error"`
}

// CommentModerationCombinationRuleConfig 描述片段内“主体词 + 行为词”组合规则。
type CommentModerationCombinationRuleConfig struct {
	ID            string   `mapstructure:"id"`
	Name          string   `mapstructure:"name"`
	Category      string   `mapstructure:"category"`
	Level         string   `mapstructure:"level"`
	Subjects      []string `mapstructure:"subjects"`
	Predicates    []string `mapstructure:"predicates"`
	SubjectRefs   []string `mapstructure:"subject_refs"`
	PredicateRefs []string `mapstructure:"predicate_refs"`
}

// CommentModerationConceptSetConfig 定义可被多条组合规则复用的规范概念集合。
// Terms 只保存规范表达，拼音、谐音和错别字仍由本地候选算法推导。
type CommentModerationConceptSetConfig struct {
	Description string   `mapstructure:"description"`
	Role        string   `mapstructure:"role"`
	Terms       []string `mapstructure:"terms"`
	Fuzzy       bool     `mapstructure:"fuzzy"`
}

// CommentModerationCategoryConfig 是审核分类的唯一注册信息。
// DefaultLevel 同时作为敏感词和组合规则未显式指定等级时的默认处置；
// FeedbackEnabled 控制该分类能否出现在管理员人工反馈选项中。
type CommentModerationCategoryConfig struct {
	Name            string `mapstructure:"name"`
	DefaultLevel    string `mapstructure:"default_level"`
	FeedbackEnabled bool   `mapstructure:"feedback_enabled"`
}

// CommentModerationRelationVocabularyConfig 定义关系分析算法依赖的中文语义角色。
// 字段名称是稳定的算法契约，字段内容属于可演进的语言策略，不应写死在 Go 代码中。
type CommentModerationRelationVocabularyConfig struct {
	NegationMarkers          []string `mapstructure:"negation_markers"`
	ImmediateNegationMarkers []string `mapstructure:"immediate_negation_markers"`
	Actors                   []string `mapstructure:"actors"`
	PersonTargets            []string `mapstructure:"person_targets"`
	ContentTargets           []string `mapstructure:"content_targets"`
	ResultConnectors         []string `mapstructure:"result_connectors"`
	PromotionActions         []string `mapstructure:"promotion_actions"`
	WeakReportingMarkers     []string `mapstructure:"weak_reporting_markers"`
	QuoteEndorsementMarkers  []string `mapstructure:"quote_endorsement_markers"`
	InterrogativePrefixes    []string `mapstructure:"interrogative_prefixes"`
	QuestionMarkers          []string `mapstructure:"question_markers"`
	FirstPersonMarkers       []string `mapstructure:"first_person_markers"`
	ContrastMarkers          []string `mapstructure:"contrast_markers"`
	ClauseBoundaryMarkers    []string `mapstructure:"clause_boundary_markers"`
	SequenceMarkers          []string `mapstructure:"sequence_markers"`
	GenericActionMarkers     []string `mapstructure:"generic_action_markers"`
	GovernanceMarkers        []string `mapstructure:"governance_markers"`
	GovernancePatterns       []string `mapstructure:"governance_patterns"`
}

// CommentModerationStanceOutcomeConfig 把一组评价结果词映射为同一种评论立场。
type CommentModerationStanceOutcomeConfig struct {
	Stance string   `mapstructure:"stance"`
	Roots  []string `mapstructure:"roots"`
}

// CommentModerationAttributeOutcomeConfig 描述“安全意识普遍较差”一类属性评价。
// Attributes 是被评价属性，Modifiers 是可选程度或范围副词，Descriptors 是负面描述。
type CommentModerationAttributeOutcomeConfig struct {
	Stance      string   `mapstructure:"stance"`
	Attributes  []string `mapstructure:"attributes"`
	Modifiers   []string `mapstructure:"modifiers"`
	Descriptors []string `mapstructure:"descriptors"`
}

// CommentModerationRiskEvaluationConfig 描述“某行为属于诈骗/违法”等评价语句的语言策略。
// 这里只提供词汇和语义角色；边界、作用域和关系推理仍由代码实现。
type CommentModerationRiskEvaluationConfig struct {
	Outcomes                 []CommentModerationStanceOutcomeConfig    `mapstructure:"outcomes"`
	AttributeOutcomes        []CommentModerationAttributeOutcomeConfig `mapstructure:"attribute_outcomes"`
	TopicSuffixes            []string                                  `mapstructure:"topic_suffixes"`
	OutcomeSuffixes          []string                                  `mapstructure:"outcome_suffixes"`
	OutcomeNegations         []string                                  `mapstructure:"outcome_negations"`
	JudgmentPredicates       []string                                  `mapstructure:"judgment_predicates"`
	DemonstrativePredicates  []string                                  `mapstructure:"demonstrative_predicates"`
	PostOutcomeRejections    []string                                  `mapstructure:"post_outcome_rejections"`
	WarningPredicates        []string                                  `mapstructure:"warning_predicates"`
	GovernanceActions        []string                                  `mapstructure:"governance_actions"`
	GovernanceModals         []string                                  `mapstructure:"governance_modals"`
	PromotionMarkers         []string                                  `mapstructure:"promotion_markers"`
	QuestionMarkers          []string                                  `mapstructure:"question_markers"`
	PromotionContrastMarkers []string                                  `mapstructure:"promotion_contrast_markers"`
	PromotionActionMarkers   []string                                  `mapstructure:"promotion_action_markers"`
}

// CommentModerationSemanticContextConfig 描述片段语义分类词族。
type CommentModerationSemanticContextConfig struct {
	ReportingMarkers         []string `mapstructure:"reporting_markers"`
	RejectionMarkers         []string `mapstructure:"rejection_markers"`
	TechnicalMarkers         []string `mapstructure:"technical_markers"`
	UnambiguousBenignMarkers []string `mapstructure:"unambiguous_benign_markers"`
	ActionableMarkers        []string `mapstructure:"actionable_markers"`
	ActionablePatterns       []string `mapstructure:"actionable_patterns"`
}

// CommentModerationAbusePolicyConfig 描述需要保留人工复核的严重攻击标记。
type CommentModerationAbusePolicyConfig struct {
	Disabled      bool     `mapstructure:"disabled"`
	SevereMarkers []string `mapstructure:"severe_markers"`
}

// CommentModerationHarmfulValuePolicyConfig 描述危险行为关系分析使用的规范概念。
// 这些列表只定义“对象、动作、语气和反证”的语义角色，不维护谐音、
// 错别字或拼音映射；文本变体由本地候选算法从规范概念自动推导。
type CommentModerationHarmfulValuePolicyConfig struct {
	Disabled            bool     `mapstructure:"disabled"`
	SelfHarmActions     []string `mapstructure:"self_harm_actions"`
	DeathWishActions    []string `mapstructure:"death_wish_actions"`
	DangerousActions    []string `mapstructure:"dangerous_actions"`
	DangerousSubstances []string `mapstructure:"dangerous_substances"`
	IngestionActions    []string `mapstructure:"ingestion_actions"`
	IncitementMarkers   []string `mapstructure:"incitement_markers"`
	IncitementSuffixes  []string `mapstructure:"incitement_suffixes"`
	IdeationMarkers     []string `mapstructure:"ideation_markers"`
	PreventionMarkers   []string `mapstructure:"prevention_markers"`
	PostventionMarkers  []string `mapstructure:"postvention_markers"`
	EducationActors     []string `mapstructure:"education_actors"`
	EducationActions    []string `mapstructure:"education_actions"`
	CriticalOutcomes    []string `mapstructure:"critical_outcomes"`
	SelfPronouns        []string `mapstructure:"self_pronouns"`
	OtherPronouns       []string `mapstructure:"other_pronouns"`
	AdditionalTargets   []string `mapstructure:"additional_targets"`
	AddressedTargets    []string `mapstructure:"addressed_targets"`
	ReferenceSuffixes   []string `mapstructure:"reference_suffixes"`
	OutcomeNegations    []string `mapstructure:"outcome_negations"`
	PromotionConflicts  []string `mapstructure:"promotion_conflicts"`
}

// CommentModerationSemanticRulesConfig 描述片段语义分类与信号修正规则。
type CommentModerationSemanticRulesConfig struct {
	Disabled           bool                                      `mapstructure:"disabled"`
	Contexts           CommentModerationSemanticContextConfig    `mapstructure:"contexts"`
	RelationVocabulary CommentModerationRelationVocabularyConfig `mapstructure:"relation_vocabulary"`
	RiskEvaluation     CommentModerationRiskEvaluationConfig     `mapstructure:"risk_evaluation"`
	AbusePolicy        CommentModerationAbusePolicyConfig        `mapstructure:"abuse_policy"`
	HarmfulValuePolicy CommentModerationHarmfulValuePolicyConfig `mapstructure:"harmful_value_policy"`
}

// CommentModerationContextAnalysisConfig 描述进程内的上下文分析策略。
// RiskConcepts 只保存规范风险概念；拼音、缩写和错别字等变体由本地算法自动生成候选。
type CommentModerationContextAnalysisConfig struct {
	Disabled      bool                `mapstructure:"disabled"`
	MaxCandidates int                 `mapstructure:"max_candidates"`
	RiskConcepts  map[string][]string `mapstructure:"risk_concepts"`
}

// CommentModerationProbabilityThresholdConfig 描述概率决策阈值。
type CommentModerationProbabilityThresholdConfig struct {
	ApproveMax    float64 `mapstructure:"approve_max"`
	RejectMin     float64 `mapstructure:"reject_min"`
	MinConfidence float64 `mapstructure:"min_confidence"`
}

// CommentModerationDecisionEngineConfig 描述证据融合决策引擎。
type CommentModerationDecisionEngineConfig struct {
	ContextAnalysis CommentModerationContextAnalysisConfig      `mapstructure:"context_analysis"`
	Thresholds      CommentModerationProbabilityThresholdConfig `mapstructure:"thresholds"`
	Calibration     CommentModerationCalibrationConfig          `mapstructure:"calibration"`
}

// CommentModerationCalibrationConfig 保存不同证据来源的初始强度。
// 这些数值是待离线校准的相对强度，不代表真实风险概率。
type CommentModerationCalibrationConfig struct {
	Version              string             `mapstructure:"version"`
	Allow                float64            `mapstructure:"allow"`
	Block                float64            `mapstructure:"block"`
	ScriptInjectionBlock float64            `mapstructure:"script_injection_block"`
	Default              float64            `mapstructure:"default"`
	Sources              map[string]float64 `mapstructure:"sources"`
}

// CommentModerationConfig 描述前台评论审核策略。
type CommentModerationConfig struct {
	// PolicyFiles 只在配置加载阶段使用。路径相对于 config 目录，按声明顺序合并；
	// 策略包中的数组会追加，映射会按键递归合并，后加载的标量覆盖前值。
	PolicyFiles       []string                                     `mapstructure:"policy_files"`
	Disabled          bool                                         `mapstructure:"disabled"`
	ReportThreshold   int64                                        `mapstructure:"report_threshold"`
	Categories        map[string]CommentModerationCategoryConfig   `mapstructure:"categories"`
	ConceptSets       map[string]CommentModerationConceptSetConfig `mapstructure:"concept_sets"`
	Lexicon           CommentModerationLexiconConfig               `mapstructure:"lexicon"`
	StructureRules    map[string]CommentModerationLevelRuleConfig  `mapstructure:"structure_rules"`
	StructurePatterns CommentModerationStructurePatternsConfig     `mapstructure:"structure_patterns"`
	CombinationRules  []CommentModerationCombinationRuleConfig     `mapstructure:"combination_rules"`
	SemanticRules     CommentModerationSemanticRulesConfig         `mapstructure:"semantic_rules"`
	BehaviorRules     CommentModerationBehaviorRulesConfig         `mapstructure:"behavior_rules"`
	DecisionEngine    CommentModerationDecisionEngineConfig        `mapstructure:"decision_engine"`
	Decision          CommentModerationDecisionConfig              `mapstructure:"decision"`
}

// RateLimitConfig 描述后端应用级限流配置。
type RateLimitConfig struct {
	AdminLogin    AdminLoginRateLimitConfig    `mapstructure:"admin_login"`
	CommentSubmit CommentSubmitRateLimitConfig `mapstructure:"comment_submit"`
	CommentReport CommentReportRateLimitConfig `mapstructure:"comment_report"`
	BugFeedback   BugFeedbackRateLimitConfig   `mapstructure:"bug_feedback"`
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
	BugFeedbackConfig       *BugFeedbackConfig       `mapstructure:"bug_feedback"`
	ArticleImageConfig      *ArticleImageConfig      `mapstructure:"article_image"`
	GuardConfig             *GuardConfig             `mapstructure:"guard"`
	RateLimitConfig         *RateLimitConfig         `mapstructure:"rate_limit"`
	CommentModerationConfig *CommentModerationConfig `mapstructure:"comment_moderation"`
}

// Replace 原子替换全部配置。调用方应先反序列化到临时 Config，成功后再替换。
// 启动后的配置文件热更新应使用 ReplaceHotReloadable，避免替换启动期配置造成误导。
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
	c.BugFeedbackConfig = next.BugFeedbackConfig
	c.ArticleImageConfig = next.ArticleImageConfig
	c.GuardConfig = next.GuardConfig
	c.RateLimitConfig = next.RateLimitConfig
	c.CommentModerationConfig = next.CommentModerationConfig
}

// ReplaceHotReloadable 只替换运行期明确支持热更新的配置段。
//
// 支持热更新的配置需要满足两个条件：
//  1. 请求处理路径通过 Config.*Snapshot 方法按需读取；
//  2. 替换配置不需要重建 logger、数据库连接池、Redis 客户端、外部客户端或 guard.Engine。
//
// 当前支持热更新：
//   - oauth：OAuth client_id / redirect_uri 等非敏感配置，secret 仍来自 env；
//   - admin_info：前台 about-me 展示信息；
//   - bug_feedback：SMTP 非敏感配置，密码仍来自 env / secret file；
//   - rate_limit：后台登录、评论、反馈等应用级限流规则；
//   - comment_moderation：评论审核策略。
//
// 仅启动期生效，修改后需要重启：
//   - log、retry、mysql、redis、article_image、guard，以及 HTTP/env
func (c *Config) ReplaceHotReloadable(next *Config) {
	if c == nil || next == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	c.OAuthConfig = next.OAuthConfig
	c.AdminInfoConfig = next.AdminInfoConfig
	c.BugFeedbackConfig = next.BugFeedbackConfig
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

// BugFeedbackSnapshot 返回 Bug 反馈配置快照。
func (c *Config) BugFeedbackSnapshot() BugFeedbackConfig {
	if c == nil {
		return BugFeedbackConfig{}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.BugFeedbackConfig == nil {
		return BugFeedbackConfig{}
	}
	return *c.BugFeedbackConfig
}

// ArticleImageCOSSnapshot 返回文章图片 COS 配置快照。
func (c *Config) ArticleImageCOSSnapshot() ArticleImageCOSConfig {
	if c == nil {
		return ArticleImageCOSConfig{}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.ArticleImageConfig == nil {
		return ArticleImageCOSConfig{}
	}
	return c.ArticleImageConfig.COS
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
	snapshot.PolicyFiles = cloneStringSlice(snapshot.PolicyFiles)
	snapshot.Categories = cloneCommentModerationCategoryConfigMap(snapshot.Categories)
	snapshot.ConceptSets = cloneCommentModerationConceptSetConfigMap(snapshot.ConceptSets)
	snapshot.Lexicon.CustomWords = cloneCommentModerationCustomWordsConfig(snapshot.Lexicon.CustomWords)
	snapshot.Lexicon.Fuzzy.CandidateWords = cloneStringSliceMap(snapshot.Lexicon.Fuzzy.CandidateWords)
	snapshot.StructureRules = cloneCommentModerationLevelRuleConfigMap(snapshot.StructureRules)
	snapshot.StructurePatterns.RiskPhrases = cloneStringSlice(snapshot.StructurePatterns.RiskPhrases)
	snapshot.StructurePatterns.RiskPatterns = cloneStringSlice(snapshot.StructurePatterns.RiskPatterns)
	snapshot.StructurePatterns.URLPatterns = cloneStringSlice(snapshot.StructurePatterns.URLPatterns)
	snapshot.StructurePatterns.ContactPatterns = cloneStringSlice(snapshot.StructurePatterns.ContactPatterns)
	snapshot.StructurePatterns.ContactLabels = cloneStringSlice(snapshot.StructurePatterns.ContactLabels)
	snapshot.StructurePatterns.NegatedContactMarkers = cloneStringSlice(
		snapshot.StructurePatterns.NegatedContactMarkers,
	)
	snapshot.StructurePatterns.BenignContactPatterns = cloneStringSlice(
		snapshot.StructurePatterns.BenignContactPatterns,
	)
	snapshot.CombinationRules = cloneCommentModerationCombinationRuleConfigSlice(snapshot.CombinationRules)
	contexts := &snapshot.SemanticRules.Contexts
	contexts.ReportingMarkers = cloneStringSlice(contexts.ReportingMarkers)
	contexts.RejectionMarkers = cloneStringSlice(contexts.RejectionMarkers)
	contexts.TechnicalMarkers = cloneStringSlice(contexts.TechnicalMarkers)
	contexts.UnambiguousBenignMarkers = cloneStringSlice(contexts.UnambiguousBenignMarkers)
	contexts.ActionableMarkers = cloneStringSlice(contexts.ActionableMarkers)
	contexts.ActionablePatterns = cloneStringSlice(contexts.ActionablePatterns)
	snapshot.SemanticRules.AbusePolicy.SevereMarkers = cloneStringSlice(
		snapshot.SemanticRules.AbusePolicy.SevereMarkers,
	)
	harmfulPolicy := &snapshot.SemanticRules.HarmfulValuePolicy
	harmfulPolicy.SelfHarmActions = cloneStringSlice(harmfulPolicy.SelfHarmActions)
	harmfulPolicy.DeathWishActions = cloneStringSlice(harmfulPolicy.DeathWishActions)
	harmfulPolicy.DangerousActions = cloneStringSlice(harmfulPolicy.DangerousActions)
	harmfulPolicy.DangerousSubstances = cloneStringSlice(harmfulPolicy.DangerousSubstances)
	harmfulPolicy.IngestionActions = cloneStringSlice(harmfulPolicy.IngestionActions)
	harmfulPolicy.IncitementMarkers = cloneStringSlice(harmfulPolicy.IncitementMarkers)
	harmfulPolicy.IncitementSuffixes = cloneStringSlice(harmfulPolicy.IncitementSuffixes)
	harmfulPolicy.IdeationMarkers = cloneStringSlice(harmfulPolicy.IdeationMarkers)
	harmfulPolicy.PreventionMarkers = cloneStringSlice(harmfulPolicy.PreventionMarkers)
	harmfulPolicy.PostventionMarkers = cloneStringSlice(harmfulPolicy.PostventionMarkers)
	harmfulPolicy.EducationActors = cloneStringSlice(harmfulPolicy.EducationActors)
	harmfulPolicy.EducationActions = cloneStringSlice(harmfulPolicy.EducationActions)
	harmfulPolicy.CriticalOutcomes = cloneStringSlice(harmfulPolicy.CriticalOutcomes)
	harmfulPolicy.SelfPronouns = cloneStringSlice(harmfulPolicy.SelfPronouns)
	harmfulPolicy.OtherPronouns = cloneStringSlice(harmfulPolicy.OtherPronouns)
	harmfulPolicy.AdditionalTargets = cloneStringSlice(harmfulPolicy.AdditionalTargets)
	harmfulPolicy.AddressedTargets = cloneStringSlice(harmfulPolicy.AddressedTargets)
	harmfulPolicy.ReferenceSuffixes = cloneStringSlice(harmfulPolicy.ReferenceSuffixes)
	harmfulPolicy.OutcomeNegations = cloneStringSlice(harmfulPolicy.OutcomeNegations)
	harmfulPolicy.PromotionConflicts = cloneStringSlice(harmfulPolicy.PromotionConflicts)
	cloneCommentModerationRelationVocabularyConfig(&snapshot.SemanticRules.RelationVocabulary)
	cloneCommentModerationRiskEvaluationConfig(&snapshot.SemanticRules.RiskEvaluation)
	snapshot.DecisionEngine.ContextAnalysis.RiskConcepts = cloneStringSliceMap(
		snapshot.DecisionEngine.ContextAnalysis.RiskConcepts,
	)
	snapshot.DecisionEngine.Calibration.Sources = cloneFloat64Map(
		snapshot.DecisionEngine.Calibration.Sources,
	)
	return snapshot
}

func cloneCommentModerationCategoryConfigMap(src map[string]CommentModerationCategoryConfig) map[string]CommentModerationCategoryConfig {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]CommentModerationCategoryConfig, len(src))
	maps.Copy(dst, src)
	return dst
}

func cloneCommentModerationConceptSetConfigMap(src map[string]CommentModerationConceptSetConfig) map[string]CommentModerationConceptSetConfig {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]CommentModerationConceptSetConfig, len(src))
	for key, value := range src {
		value.Terms = cloneStringSlice(value.Terms)
		dst[key] = value
	}
	return dst
}

func cloneCommentModerationRelationVocabularyConfig(
	cfg *CommentModerationRelationVocabularyConfig,
) {
	if cfg == nil {
		return
	}
	cfg.NegationMarkers = cloneStringSlice(cfg.NegationMarkers)
	cfg.ImmediateNegationMarkers = cloneStringSlice(cfg.ImmediateNegationMarkers)
	cfg.Actors = cloneStringSlice(cfg.Actors)
	cfg.PersonTargets = cloneStringSlice(cfg.PersonTargets)
	cfg.ContentTargets = cloneStringSlice(cfg.ContentTargets)
	cfg.ResultConnectors = cloneStringSlice(cfg.ResultConnectors)
	cfg.PromotionActions = cloneStringSlice(cfg.PromotionActions)
	cfg.WeakReportingMarkers = cloneStringSlice(cfg.WeakReportingMarkers)
	cfg.QuoteEndorsementMarkers = cloneStringSlice(cfg.QuoteEndorsementMarkers)
	cfg.InterrogativePrefixes = cloneStringSlice(cfg.InterrogativePrefixes)
	cfg.QuestionMarkers = cloneStringSlice(cfg.QuestionMarkers)
	cfg.FirstPersonMarkers = cloneStringSlice(cfg.FirstPersonMarkers)
	cfg.ContrastMarkers = cloneStringSlice(cfg.ContrastMarkers)
	cfg.ClauseBoundaryMarkers = cloneStringSlice(cfg.ClauseBoundaryMarkers)
	cfg.SequenceMarkers = cloneStringSlice(cfg.SequenceMarkers)
	cfg.GenericActionMarkers = cloneStringSlice(cfg.GenericActionMarkers)
	cfg.GovernanceMarkers = cloneStringSlice(cfg.GovernanceMarkers)
	cfg.GovernancePatterns = cloneStringSlice(cfg.GovernancePatterns)
}

func cloneCommentModerationRiskEvaluationConfig(cfg *CommentModerationRiskEvaluationConfig) {
	if cfg == nil {
		return
	}
	if len(cfg.Outcomes) > 0 {
		outcomes := make([]CommentModerationStanceOutcomeConfig, len(cfg.Outcomes))
		for index, outcome := range cfg.Outcomes {
			outcome.Roots = cloneStringSlice(outcome.Roots)
			outcomes[index] = outcome
		}
		cfg.Outcomes = outcomes
	}
	if len(cfg.AttributeOutcomes) > 0 {
		outcomes := make([]CommentModerationAttributeOutcomeConfig, len(cfg.AttributeOutcomes))
		for index, outcome := range cfg.AttributeOutcomes {
			outcome.Attributes = cloneStringSlice(outcome.Attributes)
			outcome.Modifiers = cloneStringSlice(outcome.Modifiers)
			outcome.Descriptors = cloneStringSlice(outcome.Descriptors)
			outcomes[index] = outcome
		}
		cfg.AttributeOutcomes = outcomes
	}
	cfg.TopicSuffixes = cloneStringSlice(cfg.TopicSuffixes)
	cfg.OutcomeSuffixes = cloneStringSlice(cfg.OutcomeSuffixes)
	cfg.OutcomeNegations = cloneStringSlice(cfg.OutcomeNegations)
	cfg.JudgmentPredicates = cloneStringSlice(cfg.JudgmentPredicates)
	cfg.DemonstrativePredicates = cloneStringSlice(cfg.DemonstrativePredicates)
	cfg.PostOutcomeRejections = cloneStringSlice(cfg.PostOutcomeRejections)
	cfg.WarningPredicates = cloneStringSlice(cfg.WarningPredicates)
	cfg.GovernanceActions = cloneStringSlice(cfg.GovernanceActions)
	cfg.GovernanceModals = cloneStringSlice(cfg.GovernanceModals)
	cfg.PromotionMarkers = cloneStringSlice(cfg.PromotionMarkers)
	cfg.QuestionMarkers = cloneStringSlice(cfg.QuestionMarkers)
	cfg.PromotionContrastMarkers = cloneStringSlice(cfg.PromotionContrastMarkers)
	cfg.PromotionActionMarkers = cloneStringSlice(cfg.PromotionActionMarkers)
}

func cloneCommentModerationCustomWordsConfig(
	src CommentModerationCustomWordsConfig,
) CommentModerationCustomWordsConfig {
	return CommentModerationCustomWordsConfig{
		Block:  cloneStringSliceMap(src.Block),
		Review: cloneStringSliceMap(src.Review),
	}
}

func cloneCommentModerationLevelRuleConfigMap(src map[string]CommentModerationLevelRuleConfig) map[string]CommentModerationLevelRuleConfig {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]CommentModerationLevelRuleConfig, len(src))
	maps.Copy(dst, src)
	return dst
}

func cloneFloat64Map(src map[string]float64) map[string]float64 {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]float64, len(src))
	maps.Copy(dst, src)
	return dst
}

func cloneCommentModerationCombinationRuleConfigSlice(src []CommentModerationCombinationRuleConfig) []CommentModerationCombinationRuleConfig {
	if len(src) == 0 {
		return nil
	}
	dst := make([]CommentModerationCombinationRuleConfig, len(src))
	for i, item := range src {
		dst[i] = item
		dst[i].Subjects = cloneStringSlice(item.Subjects)
		dst[i].Predicates = cloneStringSlice(item.Predicates)
		dst[i].SubjectRefs = cloneStringSlice(item.SubjectRefs)
		dst[i].PredicateRefs = cloneStringSlice(item.PredicateRefs)
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

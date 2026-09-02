package moderation

import (
	"context"
	"errors"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	appconfig "meta-api/config"
)

// ConfigSource 定义审核配置快照的读取边界，返回值是当前可供一次审核使用的独立配置。
type ConfigSource interface {
	CommentModerationSnapshot() appconfig.CommentModerationConfig
}

// BehaviorSignalFunc 接收请求上下文、原始请求、归一化文本和审核配置，返回只读行为评估。
type BehaviorSignalFunc func(context.Context, Request, NormalizedComment,
	appconfig.CommentModerationConfig) BehaviorEvaluation

// Moderator 编排词库、结构、组合、行为、上下文和证据融合等审核阶段。
type Moderator struct {
	configSource    ConfigSource
	logger          *zap.Logger
	lexicon         LexiconDetector
	behavior        *BehaviorStore
	contextAnalyzer ContextAnalyzer
	policy          policyCache
}

// NewModerator 使用 configSource（配置源）、logger（日志器）和 redis（行为存储）创建审核器，
// 返回已装配本地上下文分析器的 Moderator。
func NewModerator(configSource ConfigSource, logger *zap.Logger, redis *redis.Client) *Moderator {
	return NewModeratorWithContextAnalyzer(configSource, logger, redis, NewLocalContextAnalyzer(logger))
}

// NewModeratorWithContextAnalyzer 使用 configSource、logger、redis 和可替换的 contextAnalyzer 创建审核器，
// 返回的 Moderator 可在测试或特殊场景中注入自定义上下文分析实现。
func NewModeratorWithContextAnalyzer(configSource ConfigSource, logger *zap.Logger, redis *redis.Client,
	contextAnalyzer ContextAnalyzer,
) *Moderator {
	lexicon, err := NewSWDLexiconDetector(logger)
	if err != nil && logger != nil {
		logger.Error("create go-swd lexicon detector failed", zap.Error(err))
	}
	return &Moderator{
		configSource:    configSource,
		logger:          logger,
		lexicon:         lexicon,
		behavior:        NewBehaviorStore(redis, logger),
		contextAnalyzer: contextAnalyzer,
	}
}

// Moderate 由 m 对 req（评论请求）执行完整审核，ctx 用于取消下游操作；
// 返回 Result，并在审核中以只读方式评估行为信号。
func (m *Moderator) Moderate(ctx context.Context, req Request) Result {
	return m.ModerateWithBehavior(ctx, req, func(ctx context.Context, req Request, text NormalizedComment,
		cfg appconfig.CommentModerationConfig) BehaviorEvaluation {
		if m == nil || m.behavior == nil {
			return BehaviorEvaluation{Trace: BehaviorTrace{
				Status: "unavailable", ReadOnly: true, UnavailableReason: "behavior_store_unavailable",
			}}
		}
		return m.behavior.Evaluate(ctx, req, text, cfg)
	})
}

// ModerateWithBehavior 使用 ctx、req 和 behavior（可注入的行为评估函数）执行审核，
// 返回包含中间轨迹和最终状态的 Result。
func (m *Moderator) ModerateWithBehavior(ctx context.Context, req Request, behavior BehaviorSignalFunc) Result {
	cfg := m.config()
	if cfg.Disabled {
		return disabledResult()
	}
	compiledConfig, err := m.policy.Resolve(cfg)
	if err != nil {
		return errorResult(err, cfg)
	}
	cfg = compiledConfig
	text := Normalize(req.Content)

	signals := make([]Signal, 0, 8)
	if m == nil || m.lexicon == nil {
		return errorResult(errors.New("lexicon detector unavailable"), cfg)
	}
	lexiconSignals, err := m.lexicon.Detect(ctx, text, cfg)
	if err != nil {
		if m.logger != nil {
			m.logger.Warn("comment lexicon moderation failed", zap.Error(err))
		}
		return errorResult(err, cfg)
	}
	signals = append(signals, lexiconSignals...)
	signals = append(signals, fuzzyLexiconSignals(text, cfg)...)
	signals = append(signals, structureSignals(text, cfg)...)
	signals = append(signals, combinationSignals(text, cfg)...)
	behaviorTrace := BehaviorTrace{
		Status: "skipped", ReadOnly: true, UnavailableReason: "behavior_context_not_provided",
	}
	if behavior != nil {
		evaluation := behavior(ctx, req, text, cfg)
		signals = append(signals, evaluation.Signals...)
		behaviorTrace = evaluation.Trace
	}
	detectorSignals := append([]Signal(nil), signals...)
	signals, suppressedSignals := adjustSignalsBySemanticsWithTrace(text, signals, cfg)
	result := baselineDecision(signals, cfg)
	ruleDecision := decisionSnapshot(result)
	result.Trace = Trace{
		Clauses:           moderationClauseTrace(text, cfg),
		DetectorSignals:   detectorSignals,
		SuppressedSignals: append([]Signal(nil), suppressedSignals...),
		Behavior:          behaviorTrace,
		Decisions: DecisionFlowTrace{
			Rule:  ruleDecision,
			Final: ruleDecision,
		},
	}
	result.Trace.DecisionEngine = m.evaluateDecisionEngine(ctx, req, text,
		decisionEngineSignals(detectorSignals, signals), cfg)
	decisionFlow := result.Trace.Decisions
	result = applyDecisionEngineWithTrace(result, result.Trace.DecisionEngine, &decisionFlow)
	result.Trace.Decisions = decisionFlow
	return result
}

// decisionEngineSignals 合并 detectorSignals（原始检测信号）与 adjustedSignals 中的语义反证，
// 返回供概率决策引擎使用的信号列表。
func decisionEngineSignals(detectorSignals, adjustedSignals []Signal) []Signal {
	result := append([]Signal(nil), detectorSignals...)
	for _, signal := range adjustedSignals {
		if signal.Source == SourceSemantic && normalizeLevel(signal.Level) == LevelAllow {
			result = append(result, signal)
		}
	}
	return result
}

// moderationClauseTrace 按 cfg 的分句策略拆分 text，返回带顺序编号的 ClauseTrace 列表。
func moderationClauseTrace(text NormalizedComment, cfg appconfig.CommentModerationConfig) []ClauseTrace {
	clauses := semanticClauses(text, cfg)
	trace := make([]ClauseTrace, 0, len(clauses))
	for index, clause := range clauses {
		trace = append(trace, ClauseTrace{
			ID:   index + 1,
			Text: clause,
		})
	}
	return trace
}

// RecordBehavior 使用 ctx 将 req 代表的已落库评论行为写入 m 的行为存储；该方法无返回值。
func (m *Moderator) RecordBehavior(ctx context.Context, req Request) {
	if m == nil || m.behavior == nil {
		return
	}
	m.behavior.Record(ctx, req, m.config())
}

// config 从 m 的配置源读取快照并补齐默认值，返回本次审核使用的配置。
func (m *Moderator) config() appconfig.CommentModerationConfig {
	var cfg appconfig.CommentModerationConfig
	if m != nil && m.configSource != nil {
		cfg = m.configSource.CommentModerationSnapshot()
	}
	ApplyDefaults(&cfg)
	return cfg
}

// ApplyDefaults 对 cfg 中可缺省的检测器、阈值和行为规则填充安全默认值；函数原地修改 cfg，无返回值。
func ApplyDefaults(cfg *appconfig.CommentModerationConfig) {
	if cfg == nil {
		return
	}
	if cfg.Lexicon.Provider == "" {
		cfg.Lexicon.Provider = "go_swd"
	}
	cfg.Lexicon.UseBuiltin = true
	cfg.Lexicon.StrictBuiltinMatch = true
	if cfg.Decision.DefaultOnError == "" {
		cfg.Decision.DefaultOnError = "pending"
	}
	if cfg.DecisionEngine.ContextAnalysis.MaxCandidates <= 0 {
		cfg.DecisionEngine.ContextAnalysis.MaxCandidates = 16
	}
	if cfg.DecisionEngine.Thresholds.ApproveMax <= 0 {
		cfg.DecisionEngine.Thresholds.ApproveMax = 0.2
	}
	if cfg.DecisionEngine.Thresholds.RejectMin <= cfg.DecisionEngine.Thresholds.ApproveMax {
		cfg.DecisionEngine.Thresholds.RejectMin = 0.9
	}
	if cfg.DecisionEngine.Thresholds.MinConfidence <= 0 {
		cfg.DecisionEngine.Thresholds.MinConfidence = 0.7
	}
	if cfg.StructureRules == nil {
		cfg.StructureRules = map[string]appconfig.CommentModerationLevelRuleConfig{}
	}
	for name, policy := range cfg.Categories {
		if level := normalizeLevel(policy.DefaultLevel); level != "" {
			if _, exists := cfg.StructureRules[name]; !exists {
				cfg.StructureRules[name] = appconfig.CommentModerationLevelRuleConfig{Level: level}
			}
		}
	}
	defaultStructureLevels := map[string]string{
		"url":              LevelReview,
		"decoded_url":      LevelReview,
		"contact":          LevelReview,
		"script_injection": LevelBlock,
		"text_quality":     LevelReview,
		"risk_phrase":      LevelReview,
	}
	for name, level := range defaultStructureLevels {
		if _, ok := cfg.StructureRules[name]; !ok {
			cfg.StructureRules[name] = appconfig.CommentModerationLevelRuleConfig{Level: level}
		}
	}
	_ = userRule(*cfg)
	_ = ipRule(*cfg)
	_ = duplicateRule(*cfg)
}

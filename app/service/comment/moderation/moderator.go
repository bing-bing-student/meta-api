package moderation

import (
	"context"
	"errors"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	appconfig "meta-api/config"
)

type ConfigSource interface {
	CommentModerationSnapshot() appconfig.CommentModerationConfig
}

type BehaviorSignalFunc func(context.Context, Request, NormalizedComment,
	appconfig.CommentModerationConfig) BehaviorEvaluation

type Moderator struct {
	configSource    ConfigSource
	logger          *zap.Logger
	lexicon         LexiconDetector
	behavior        *BehaviorStore
	contextAnalyzer ContextAnalyzer
	policy          policyCache
}

func NewModerator(configSource ConfigSource, logger *zap.Logger, redis *redis.Client) *Moderator {
	return NewModeratorWithContextAnalyzer(configSource, logger, redis, NewLocalContextAnalyzer(logger))
}

// NewModeratorWithContextAnalyzer makes the local context boundary injectable for tests.
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
		Clauses:           moderationClauseTrace(text),
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

func decisionEngineSignals(detectorSignals, adjustedSignals []Signal) []Signal {
	result := append([]Signal(nil), detectorSignals...)
	for _, signal := range adjustedSignals {
		if signal.Source == SourceSemantic && normalizeLevel(signal.Level) == LevelAllow {
			result = append(result, signal)
		}
	}
	return result
}

func moderationClauseTrace(text NormalizedComment) []ClauseTrace {
	clauses := semanticClauses(text)
	trace := make([]ClauseTrace, 0, len(clauses))
	for index, clause := range clauses {
		trace = append(trace, ClauseTrace{
			ID:   index + 1,
			Text: clause,
		})
	}
	return trace
}

func (m *Moderator) RecordBehavior(ctx context.Context, req Request) {
	if m == nil || m.behavior == nil {
		return
	}
	m.behavior.Record(ctx, req, m.config())
}

func (m *Moderator) config() appconfig.CommentModerationConfig {
	var cfg appconfig.CommentModerationConfig
	if m != nil && m.configSource != nil {
		cfg = m.configSource.CommentModerationSnapshot()
	}
	ApplyDefaults(&cfg)
	return cfg
}

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

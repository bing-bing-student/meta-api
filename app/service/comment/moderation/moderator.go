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

type BehaviorSignalFunc func(context.Context, Request, NormalizedComment, appconfig.CommentModerationConfig) []Signal

type Moderator struct {
	configSource ConfigSource
	logger       *zap.Logger
	lexicon      LexiconDetector
	behavior     *BehaviorStore
	policy       policyCache
}

func NewModerator(configSource ConfigSource, logger *zap.Logger, redis *redis.Client) *Moderator {
	lexicon, err := NewSWDLexiconDetector(logger)
	if err != nil && logger != nil {
		logger.Error("create go-swd lexicon detector failed", zap.Error(err))
	}
	return &Moderator{
		configSource: configSource,
		logger:       logger,
		lexicon:      lexicon,
		behavior:     NewBehaviorStore(redis, logger),
	}
}

func (m *Moderator) Moderate(ctx context.Context, req Request) Result {
	return m.ModerateWithBehavior(ctx, req, func(ctx context.Context, req Request, text NormalizedComment,
		cfg appconfig.CommentModerationConfig) []Signal {
		if m == nil || m.behavior == nil {
			return nil
		}
		return m.behavior.Signals(ctx, req, text, cfg)
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
	if behavior != nil {
		signals = append(signals, behavior(ctx, req, text, cfg)...)
	}
	detectorSignals := append([]Signal(nil), signals...)
	signals, suppressedSignals := adjustSignalsBySemanticsWithTrace(text, signals, cfg)
	result := decide(signals, cfg)
	result.Trace = Trace{
		Clauses:           moderationClauseTrace(text),
		DetectorSignals:   detectorSignals,
		SuppressedSignals: append([]Signal(nil), suppressedSignals...),
		BehaviorEvaluated: behavior != nil,
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
	if cfg.Decision.Score.Pending <= 0 {
		cfg.Decision.Score.Pending = defaultPendingScore
	}
	if cfg.Decision.Score.Reject <= cfg.Decision.Score.Pending {
		cfg.Decision.Score.Reject = defaultRejectScore
	}
	if cfg.Decision.CategoryOverrides == nil {
		cfg.Decision.CategoryOverrides = map[string]appconfig.CommentModerationCategoryDecisionConfig{}
	}
	defaultOverrides := map[string]string{
		"sexual":     LevelBlock,
		"gambling":   LevelBlock,
		"drugs":      LevelBlock,
		"political":  LevelReview,
		"violence":   LevelReview,
		"abuse":      LevelReview,
		"hate":       LevelReview,
		"spam_fraud": LevelReview,
		"sensitive":  LevelReview,
		"custom":     LevelReview,
	}
	for category, level := range defaultOverrides {
		if _, ok := cfg.Decision.CategoryOverrides[category]; !ok {
			cfg.Decision.CategoryOverrides[category] = appconfig.CommentModerationCategoryDecisionConfig{Level: level}
		}
	}
	if cfg.StructureRules == nil {
		cfg.StructureRules = map[string]appconfig.CommentModerationLevelRuleConfig{}
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

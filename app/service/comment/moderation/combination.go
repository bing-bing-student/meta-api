package moderation

import (
	"strings"

	appconfig "meta-api/config"
)

// combinationSignals 在同一非良性分句内组合配置的主体词和谓词，生成上下文风险信号。
// 输入 text 是归一化评论，cfg 提供组合规则和评分策略；返回值是已按规则与证据去重的信号集合。
func combinationSignals(text NormalizedComment, cfg appconfig.CommentModerationConfig) []Signal {
	rules := cfg.CombinationRules
	signals := make([]Signal, 0)
	seen := make(map[string]struct{})
	for _, rule := range rules {
		ruleID := strings.TrimSpace(rule.ID)
		if ruleID == "" {
			ruleID = strings.TrimSpace(rule.Name)
		}
		if ruleID == "" {
			continue
		}
		category := strings.TrimSpace(rule.Category)
		if category == "" {
			category = ruleID
		}
		level := normalizeLevel(rule.Level)
		if level == "" {
			level = LevelReview
		}
		for clauseIndex, clause := range semanticClauses(text, cfg) {
			if isBenignSemanticClause(clause, cfg) {
				continue
			}
			views := []string{clause.Compact}
			for _, subject := range containsAnyCompact(views, rule.Subjects) {
				for _, predicate := range containsAnyCompact(views, rule.Predicates) {
					if subject == predicate ||
						!relationPredicateDirectionValid(clause.Normalized, subject, predicate, cfg) {
						continue
					}
					evidence := subject + "+" + predicate
					key := ruleID + ":" + evidence
					if _, ok := seen[key]; ok {
						continue
					}
					seen[key] = struct{}{}
					signals = append(signals, Signal{
						Source:   SourceContext,
						Category: category,
						Level:    level,
						Score:    evidenceStrengthScore(SourceContext, category, ruleID, level, cfg),
						Reason:   formatReason(SourceContext, category, level, ruleID+":"+evidence),
						Evidence: evidence,
						RuleID:   ruleID,
						Clause:   clauseIndex + 1,
					})
				}
			}
		}
	}
	return signals
}

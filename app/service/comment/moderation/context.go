package moderation

import (
	"strings"

	appconfig "meta-api/config"
)

func contextSignals(text NormalizedComment, cfg appconfig.CommentModerationConfig) []Signal {
	rules := cfg.ContextRules

	views := []string{text.Compact}
	if text.PinyinFolded != "" && text.PinyinFolded != text.Compact {
		views = append(views, text.PinyinFolded)
	}

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
		for _, subject := range containsAnyCompact(views, rule.Subjects) {
			for _, predicate := range containsAnyCompact(views, rule.Predicates) {
				if subject == predicate {
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
					Score:    scoreForLevel(level, cfg),
					Reason:   formatReason(SourceContext, category, level, ruleID+":"+evidence),
					Evidence: evidence,
					RuleID:   ruleID,
				})
			}
		}
	}
	return signals
}

package moderation

import (
	"strings"

	appconfig "meta-api/config"
)

// adjustSignalsBySemantics 使用分句语境抑制误报信号，并省略抑制明细。
// 输入 text 是归一化评论、signals 是原始信号、cfg 是语义配置；返回调整后的信号集合。
func adjustSignalsBySemantics(text NormalizedComment, signals []Signal,
	cfg appconfig.CommentModerationConfig) []Signal {
	adjusted, _ := adjustSignalsBySemanticsWithTrace(text, signals, cfg)
	return adjusted
}

// adjustSignalsBySemanticsWithTrace 按局部分句判断每个信号是否应被良性语境抑制。
// 输入 text、signals、cfg 分别为评论、原始信号和配置；返回调整后信号及被抑制的原信号。
func adjustSignalsBySemanticsWithTrace(text NormalizedComment, signals []Signal,
	cfg appconfig.CommentModerationConfig) ([]Signal, []Signal) {
	if len(signals) == 0 || cfg.SemanticRules.Disabled {
		return signals, nil
	}

	adjusted := make([]Signal, 0, len(signals))
	suppressedSignals := make([]Signal, 0)
	suppressed := make([]string, 0)
	for _, signal := range signals {
		if shouldSuppressSignalLocally(text, signal, cfg) {
			suppressedSignals = append(suppressedSignals, signal)
			suppressed = append(suppressed, signal.Evidence)
			continue
		}
		adjusted = append(adjusted, signal)
	}
	if len(suppressed) == 0 {
		return signals, nil
	}

	adjusted = append(adjusted, Signal{
		Source:   SourceSemantic,
		Category: "benign_context",
		Level:    LevelAllow,
		Evidence: strings.Join(compactSemanticEvidence(suppressed), ","),
		RuleID:   "benign_context",
	})
	return adjusted, suppressedSignals
}

// shouldSuppressSignalLocally 判断 signal 对应分句是否全部为良性语境。
// 输入 text 用于定位证据、cfg 提供语义规则；返回 true 表示可抑制该信号，行为及硬结构信号不会被抑制。
func shouldSuppressSignalLocally(text NormalizedComment, signal Signal,
	cfg appconfig.CommentModerationConfig) bool {
	if signal.Source == SourceBehavior {
		return false
	}
	if signal.Source == SourceStructure {
		switch signal.RuleID {
		case "decoded_url", "script_injection", "text_quality":
			return false
		}
	}

	clauses := relevantSignalClauses(text, signal, cfg)
	if len(clauses) == 0 {
		return false
	}
	abusePolicy := cfg.SemanticRules.AbusePolicy
	if signal.Category == "abuse" && !abusePolicy.Disabled && len(abusePolicy.SevereMarkers) > 0 {
		for _, clause := range clauses {
			if isSevereAbuseClause(clause, abusePolicy.SevereMarkers) {
				return false
			}
		}
		return true
	}
	for _, clause := range clauses {
		if !isBenignSemanticClause(clause, cfg) {
			return false
		}
	}
	return true
}

// isSevereAbuseClause 判断 clause 的紧凑文本或形近骨架是否包含严重辱骂 markers；返回对应结果。
func isSevereAbuseClause(clause NormalizedComment, markers []string) bool {
	return containsAnyNormalized(clause.Compact, markers) ||
		containsAnyNormalized(clause.Confusable, markers)
}

// relevantSignalClauses 定位 signal 实际对应的语义分句。
// 输入 text 是完整评论、signal 是待定位信号、cfg 提供匹配规则；返回相关分句，无法定位时返回 nil。
func relevantSignalClauses(text NormalizedComment, signal Signal,
	cfg appconfig.CommentModerationConfig) []NormalizedComment {
	clauses := semanticClauses(text, cfg)
	if signal.Clause > 0 && signal.Clause <= len(clauses) {
		return []NormalizedComment{clauses[signal.Clause-1]}
	}
	switch signal.Source {
	case SourceLexicon, SourceContext:
		return clausesContainingEvidence(text, signal.Evidence, cfg)
	case SourceStructure:
		matches := make([]NormalizedComment, 0, len(clauses))
		for _, clause := range clauses {
			matched := false
			switch signal.RuleID {
			case "url":
				matched = matchesConfiguredURL(clause.Normalized, cfg)
			case "contact":
				matched = phoneRegexp.MatchString(clause.Normalized) ||
					matchesConfiguredContact(clause.Normalized, cfg)
			case "risk_phrase":
				matched = matchesRiskPhrase(clause, cfg)
			}
			if matched {
				matches = append(matches, clause)
			}
		}
		return matches
	default:
		return nil
	}
}

// compactSemanticEvidence 清理 values 中的空证据并按首次出现顺序去重；返回可用于信号展示的证据列表。
func compactSemanticEvidence(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

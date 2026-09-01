package moderation

import (
	"strings"

	appconfig "meta-api/config"
)

func adjustSignalsBySemantics(text NormalizedComment, signals []Signal,
	cfg appconfig.CommentModerationConfig) []Signal {
	adjusted, _ := adjustSignalsBySemanticsWithTrace(text, signals, cfg)
	return adjusted
}

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

func isSevereAbuseClause(clause NormalizedComment, markers []string) bool {
	return containsAnyNormalized(clause.Compact, markers) ||
		containsAnyNormalized(clause.Confusable, markers)
}

func relevantSignalClauses(text NormalizedComment, signal Signal,
	cfg appconfig.CommentModerationConfig) []NormalizedComment {
	clauses := semanticClauses(text)
	if signal.Clause > 0 && signal.Clause <= len(clauses) {
		return []NormalizedComment{clauses[signal.Clause-1]}
	}
	switch signal.Source {
	case SourceLexicon, SourceContext:
		return clausesContainingEvidence(text, signal.Evidence)
	case SourceStructure:
		matches := make([]NormalizedComment, 0, len(clauses))
		for _, clause := range clauses {
			matched := false
			switch signal.RuleID {
			case "url":
				matched = domainRegexp.MatchString(clause.Normalized) ||
					obfuscatedDomainRegexp.MatchString(clause.Normalized)
			case "contact":
				matched = phoneRegexp.MatchString(clause.Normalized) ||
					accountRegexp.MatchString(clause.Normalized) ||
					contactIntentRegexp.MatchString(clause.Normalized) ||
					emailObfuscationRegexp.MatchString(clause.Normalized)
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

package moderation

import (
	"regexp"
	"strings"
	"sync"
	"unicode"

	appconfig "meta-api/config"
)

var semanticPatternCache sync.Map

func semanticClauses(text NormalizedComment) []NormalizedComment {
	parts := splitSemanticText(text.Raw)
	if len(parts) == 0 {
		return []NormalizedComment{text}
	}

	clauses := make([]NormalizedComment, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		clauses = append(clauses, Normalize(part))
	}
	if len(clauses) == 0 {
		return []NormalizedComment{text}
	}
	return clauses
}

func splitSemanticText(value string) []string {
	runes := []rune(value)
	parts := make([]string, 0, 4)
	start := 0
	for index, r := range runes {
		if !isSemanticSeparator(r) || isURLColon(runes, index) ||
			isContactColon(runes, index) || isDomainDot(runes, index) {
			continue
		}
		if part := strings.TrimSpace(string(runes[start:index])); part != "" {
			parts = append(parts, part)
		}
		start = index + 1
	}
	if part := strings.TrimSpace(string(runes[start:])); part != "" {
		parts = append(parts, part)
	}
	return parts
}

func isContactColon(runes []rune, index int) bool {
	if index < 1 || runes[index] != ':' && runes[index] != '：' {
		return false
	}
	start := index - 8
	if start < 0 {
		start = 0
	}
	prefix := compactText(strings.ToLower(string(runes[start:index])))
	for _, marker := range []string{"vx", "v信", "微信", "微信号", "wx", "qq", "企鹅", "账号", "联系方式"} {
		if strings.HasSuffix(prefix, marker) {
			return true
		}
	}
	return false
}

func isSemanticSeparator(r rune) bool {
	switch r {
	case '。', '！', '？', '；', '：', '.', '!', '?', ';', ':', '\n', '\r':
		return true
	default:
		return false
	}
}

func isDomainDot(runes []rune, index int) bool {
	if index <= 0 || index+1 >= len(runes) || runes[index] != '.' {
		return false
	}
	return isDomainRune(runes[index-1]) && isDomainRune(runes[index+1])
}

func isDomainRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_'
}

func isURLColon(runes []rune, index int) bool {
	if index < 1 || runes[index] != ':' {
		return false
	}
	start := index - 8
	if start < 0 {
		start = 0
	}
	prefix := strings.ToLower(string(runes[start:index]))
	return strings.HasSuffix(prefix, "http") ||
		strings.HasSuffix(prefix, "https") ||
		strings.HasSuffix(prefix, "hxxp") ||
		strings.HasSuffix(prefix, "hxxps") ||
		strings.HasSuffix(prefix, "javascript")
}

func isBenignSemanticClause(clause NormalizedComment, cfg appconfig.CommentModerationConfig) bool {
	value := clause.Compact
	if value == "" {
		return true
	}
	contexts := cfg.SemanticRules.Contexts
	if matchesAnySemanticPattern(clause.Normalized, contexts.ActionablePatterns) {
		return false
	}
	if containsAnyNormalized(value, contexts.UnambiguousBenignMarkers) {
		return true
	}
	if containsAnyNormalized(value, contexts.ActionableMarkers) {
		return false
	}
	return containsAnyNormalized(value, contexts.ReportingMarkers) ||
		containsAnyNormalized(value, contexts.RejectionMarkers) ||
		containsAnyNormalized(value, contexts.TechnicalMarkers)
}

func containsAnyNormalized(value string, candidates []string) bool {
	for _, candidate := range candidates {
		if candidate != "" && strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}

func matchesAnySemanticPattern(value string, patterns []string) bool {
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		compiled, ok := semanticPatternCache.Load(pattern)
		if !ok {
			re, err := regexp.Compile(pattern)
			if err != nil {
				continue
			}
			compiled, _ = semanticPatternCache.LoadOrStore(pattern, re)
		}
		if compiled.(*regexp.Regexp).MatchString(value) {
			return true
		}
	}
	return false
}

func clausesContainingEvidence(text NormalizedComment, evidence string) []NormalizedComment {
	terms := evidenceTerms(evidence)
	clauses := semanticClauses(text)
	if len(terms) == 0 {
		return clauses
	}

	matches := make([]NormalizedComment, 0, len(clauses))
	for _, clause := range clauses {
		matched := true
		for _, term := range terms {
			if !strings.Contains(clause.Compact, term) &&
				!strings.Contains(clause.PinyinFolded, term) {
				matched = false
				break
			}
		}
		if matched {
			matches = append(matches, clause)
		}
	}
	return matches
}

func evidenceTerms(evidence string) []string {
	if evidence == "" {
		return nil
	}
	fields := strings.FieldsFunc(evidence, func(r rune) bool {
		return r == '+' || r == ',' || r == ':' || unicode.IsSpace(r)
	})
	terms := make([]string, 0, len(fields))
	for _, field := range fields {
		field = compactText(normalizeText(field))
		if field == "" || field == "url" || field == "contact" || field == "riskphrase" {
			continue
		}
		terms = append(terms, field)
	}
	return terms
}

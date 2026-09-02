package moderation

import (
	"regexp"
	"strings"
	"sync"
	"unicode"

	appconfig "meta-api/config"
)

var semanticPatternCache sync.Map

// semanticClauses 将 text 按语义边界切成独立分句，并对每个分句重新归一化。
// 输入 text 是完整评论，cfg 提供分句边界词；返回值至少包含一个可分析的归一化分句。
func semanticClauses(text NormalizedComment, cfg appconfig.CommentModerationConfig) []NormalizedComment {
	parts := splitSemanticText(text.Raw, cfg)
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

// splitSemanticText 扫描 value 中的标点和配置边界词，切分语义文本，同时保留 URL、域名和联系方式内部的符号。
// 输入 value 是原始文本，cfg 提供联系方式标签及边界词；返回值是去除首尾空白后的非空分句。
func splitSemanticText(value string, cfg appconfig.CommentModerationConfig) []string {
	runes := []rune(value)
	parts := make([]string, 0, 4)
	start := 0
	for index, r := range runes {
		if (!isSemanticSeparator(r) && !isScopedCommaSeparator(runes, index, cfg)) || isURLColon(runes, index) ||
			isContactColon(runes, index, cfg) || isDomainDot(runes, index) {
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

// isScopedCommaSeparator 判断指定逗号后是否紧跟配置的分句边界词。
// 输入 runes 是全文字符、index 是待判断位置、cfg 是审核配置；返回 true 表示该逗号应作为分句符。
func isScopedCommaSeparator(runes []rune, index int, cfg appconfig.CommentModerationConfig) bool {
	if index < 0 || index >= len(runes) || runes[index] != '，' && runes[index] != ',' {
		return false
	}
	end := index + 10
	if end > len(runes) {
		end = len(runes)
	}
	suffix := compactText(normalizeText(string(runes[index+1 : end])))
	for _, marker := range cfg.SemanticRules.RelationVocabulary.ClauseBoundaryMarkers {
		if strings.HasPrefix(suffix, marker) {
			return true
		}
	}
	return false
}

// isContactColon 判断指定冒号是否属于“联系方式标签:账号”结构。
// 输入 runes 是全文字符、index 是冒号位置、cfg 提供联系方式标签；返回 true 表示冒号不应切断该结构。
func isContactColon(runes []rune, index int, cfg appconfig.CommentModerationConfig) bool {
	if index < 1 || runes[index] != ':' && runes[index] != '：' {
		return false
	}
	start := index - 8
	if start < 0 {
		start = 0
	}
	prefix := compactText(strings.ToLower(string(runes[start:index])))
	for _, marker := range cfg.StructurePatterns.ContactLabels {
		if strings.HasSuffix(prefix, marker) {
			return true
		}
	}
	return false
}

// isSemanticSeparator 判断字符 r 是否属于默认语义分隔符；返回 true 表示可在此处切分。
func isSemanticSeparator(r rune) bool {
	switch r {
	case '。', '！', '？', '；', '：', '.', '!', '?', ';', ':', '\n', '\r':
		return true
	default:
		return false
	}
}

// isDomainDot 判断 index 处的点是否连接域名字符。
// 输入 runes 是全文字符、index 是点的位置；返回 true 表示该点属于域名而非句号。
func isDomainDot(runes []rune, index int) bool {
	if index <= 0 || index+1 >= len(runes) || runes[index] != '.' {
		return false
	}
	return isDomainRune(runes[index-1]) && isDomainRune(runes[index+1])
}

// isDomainRune 判断字符 r 是否可作为域名点号两侧的字符；返回 true 表示可用。
func isDomainRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_'
}

// isURLColon 判断 index 处的冒号是否属于常见 URL 协议前缀。
// 输入 runes 是全文字符、index 是冒号位置；返回 true 表示该冒号不应作为分句边界。
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

// isBenignSemanticClause 综合行动、说明、拒绝和技术语境判断 clause 是否为低风险分句。
// 输入 clause 是归一化分句，cfg 提供语义规则；返回 true 表示该分句可作为良性上下文处理。
func isBenignSemanticClause(clause NormalizedComment, cfg appconfig.CommentModerationConfig) bool {
	value := clause.Compact
	if value == "" {
		return true
	}
	contexts := cfg.SemanticRules.Contexts
	if matchesAnySemanticPattern(clause.Normalized, contexts.ActionablePatterns) {
		return false
	}
	if isRiskEvaluationSemanticClause(clause, cfg) {
		return true
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

// containsAnyNormalized 判断 value 是否包含 candidates 中任一非空词项。
// 返回 true 表示至少命中一个候选词，空候选会被忽略。
func containsAnyNormalized(value string, candidates []string) bool {
	for _, candidate := range candidates {
		if candidate != "" && strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}

// matchesAnySemanticPattern 判断 value 是否命中 patterns 中任一正则表达式。
// 输入 patterns 是配置的正则列表；返回 true 表示至少产生一个非空匹配。
func matchesAnySemanticPattern(value string, patterns []string) bool {
	return len(semanticPatternMatches(value, patterns)) > 0
}

// semanticPatternMatches 执行并缓存 patterns 中的合法正则表达式。
// 输入 value 是待匹配文本；返回值汇总所有非空匹配片段，非法正则会被跳过。
func semanticPatternMatches(value string, patterns []string) []string {
	result := make([]string, 0, 2)
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
		for _, match := range compiled.(*regexp.Regexp).FindAllString(value, -1) {
			if match != "" {
				result = append(result, match)
			}
		}
	}
	return result
}

// clausesContainingEvidence 找出同时包含证据全部有效词项的语义分句。
// 输入 text 是完整评论、evidence 是组合证据、cfg 是审核配置；返回匹配分句，证据无词项时返回全部分句。
func clausesContainingEvidence(text NormalizedComment, evidence string,
	cfg appconfig.CommentModerationConfig,
) []NormalizedComment {
	terms := evidenceTerms(evidence)
	clauses := semanticClauses(text, cfg)
	if len(terms) == 0 {
		return clauses
	}

	matches := make([]NormalizedComment, 0, len(clauses))
	for _, clause := range clauses {
		matched := true
		for _, term := range terms {
			if !strings.Contains(clause.Compact, term) {
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

// evidenceTerms 将 evidence 拆成用于分句定位的归一化词项。
// 输入 evidence 是规则证据字符串；返回值会过滤通用结构标签及空词项。
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

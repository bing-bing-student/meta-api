package moderation

import (
	"regexp"
	"strings"
	"unicode"

	appconfig "meta-api/config"
)

var (
	scriptRegexp           = regexp.MustCompile(`(?i)(<\s*script\b|javascript\s*:)`)
	phoneRegexp            = regexp.MustCompile(`\b1[3-9]\d{9}\b`)
	domainRegexp           = regexp.MustCompile(`(?i)(https?\s*:\s*/\s*/|www\s*\.|[\p{Han}a-z0-9][\p{Han}a-z0-9._-]*\s*\.\s*(com|cn|net|org|top|xyz|shop|vip|cc|io|me)\b)`)
	obfuscatedDomainRegexp = regexp.MustCompile(`(?i)(hxxps?\s*:\s*/\s*/|[a-z0-9-]+\s*(点|\[\s*\.\s*\])\s*(com|cn|net|org|top|xyz|shop|vip|cc|io|me)\b)`)
	accountRegexp          = regexp.MustCompile(`(?i)(加|群|qq|vx|v信|微信|筘|扣|抠|薇|威|扫码|二维码|企鹅群)[^\n]{0,12}\d[\d\s]{4,}|(\b(v|vx|qq)\b|微信|微信号|v信)\s*[:：]\s*[a-z0-9_-]{4,}|(\b(v|vx|qq)\b|微信|微信号|v信)\s*[:：]\s*(?:[a-z0-9][\s_-]*){4,}`)
	contactIntentRegexp    = regexp.MustCompile(`(?i)(加\s*(微信|vx|v信|qq|群|好友|筘|扣|抠|薇|威)|v我|联系\s*我|私信|进群|扫码|二维码|绿泡泡|绿信|微信号|账号写图片|联系方式)`)
	emailObfuscationRegexp = regexp.MustCompile(`(?i)\b[a-z0-9._%+-]+\s+at\s+[a-z0-9.-]+\s+dot\s+[a-z]{2,}\b`)
)

func structureSignals(text NormalizedComment, cfg appconfig.CommentModerationConfig) []Signal {
	signals := make([]Signal, 0, 4)
	if hasRiskyScriptClause(text, cfg) {
		signals = append(signals, newStructureSignal("script_injection", "script", cfg))
	}
	if hasRiskyURLClause(text, cfg) {
		signals = append(signals, newStructureSignal("url", "url", cfg))
	}
	if hasRiskyContactClause(text, cfg) {
		signals = append(signals, newStructureSignal("contact", "contact", cfg))
	}
	if matchesRiskPhrase(text, cfg) {
		signals = append(signals, newStructureSignal("risk_phrase", "risk_phrase", cfg))
	}
	for _, decoded := range text.DecodedTexts {
		signals = append(signals, newStructureSignal("decoded_url", decoded, cfg))
	}
	if signal, ok := textQualitySignal(text, cfg); ok {
		signals = append(signals, signal)
	}
	return signals
}

func matchesRiskPhrase(text NormalizedComment, cfg appconfig.CommentModerationConfig) bool {
	return containsAnyNormalized(text.Compact, cfg.StructurePatterns.RiskPhrases) ||
		matchesAnySemanticPattern(text.Normalized, cfg.StructurePatterns.RiskPatterns)
}

func hasRiskyScriptClause(text NormalizedComment, cfg appconfig.CommentModerationConfig) bool {
	for _, clause := range semanticClauses(text) {
		if scriptRegexp.MatchString(clause.Normalized) && !isBenignSemanticClause(clause, cfg) {
			return true
		}
	}
	return false
}

func hasRiskyURLClause(text NormalizedComment, cfg appconfig.CommentModerationConfig) bool {
	if (domainRegexp.MatchString(text.Normalized) ||
		obfuscatedDomainRegexp.MatchString(text.Normalized)) &&
		!allSemanticClausesBenign(text, cfg) {
		return true
	}
	for _, clause := range semanticClauses(text) {
		if !domainRegexp.MatchString(clause.Normalized) &&
			!obfuscatedDomainRegexp.MatchString(clause.Normalized) {
			continue
		}
		if !isBenignSemanticClause(clause, cfg) {
			return true
		}
	}
	return false
}

func hasRiskyContactClause(text NormalizedComment, cfg appconfig.CommentModerationConfig) bool {
	fullMatch := phoneRegexp.MatchString(text.Normalized) ||
		accountRegexp.MatchString(text.Normalized) ||
		contactIntentRegexp.MatchString(text.Normalized) ||
		emailObfuscationRegexp.MatchString(text.Normalized) ||
		len(matchContactShapes(text.Normalized)) > 0
	if fullMatch && !allSemanticClausesBenign(text, cfg) {
		return true
	}
	for _, clause := range semanticClauses(text) {
		value := clause.Normalized
		matched := phoneRegexp.MatchString(value) ||
			accountRegexp.MatchString(value) ||
			contactIntentRegexp.MatchString(value) ||
			emailObfuscationRegexp.MatchString(value) ||
			len(matchContactShapes(value)) > 0
		if matched && !isNegatedContactMention(value) &&
			!isBenignContactMention(value) &&
			!isBenignSemanticClause(clause, cfg) {
			return true
		}
	}
	return false
}

func allSemanticClausesBenign(text NormalizedComment, cfg appconfig.CommentModerationConfig) bool {
	clauses := semanticClauses(text)
	if len(clauses) == 0 {
		return false
	}
	for _, clause := range clauses {
		if !isBenignSemanticClause(clause, cfg) {
			return false
		}
	}
	return true
}

func isNegatedContactMention(value string) bool {
	if strings.Contains(value, "不包含联系方式") ||
		strings.Contains(value, "没有联系方式") ||
		strings.Contains(value, "不留联系方式") ||
		strings.Contains(value, "无需联系方式") {
		return true
	}
	return containsAnyString(value, []string{"不需要", "无需", "不要", "不应", "禁止", "别把"}) &&
		contactIntentRegexp.MatchString(value)
}

func isBenignContactMention(value string) bool {
	return strings.Contains(value, "联系方式识别测试") ||
		strings.Contains(value, "联系方式测试") ||
		(strings.Contains(value, "不要把") && strings.Contains(value, "当成违规")) ||
		(strings.Contains(value, "别把") && strings.Contains(value, "判违规")) ||
		(strings.Contains(value, "正常讨论") && strings.Contains(value, "误杀")) ||
		(containsAnyString(value, []string{"测试数据", "测试号码", "示例账号", "正则", "反垃圾样本"}) &&
			containsAnyString(value, []string{"微信", "账号", "手机号", "号码", "邮件", " at "}))
}

func newStructureSignal(ruleID, evidence string, cfg appconfig.CommentModerationConfig) Signal {
	level := structureRuleLevel(ruleID, cfg)
	return Signal{
		Source:   SourceStructure,
		Category: ruleID,
		Level:    level,
		Score:    scoreForSignal(SourceStructure, ruleID, ruleID, level, cfg),
		Reason:   formatReason(SourceStructure, ruleID, level, evidence),
		Evidence: evidence,
		RuleID:   ruleID,
	}
}

func structureRuleLevel(ruleID string, cfg appconfig.CommentModerationConfig) string {
	if rule, ok := cfg.StructureRules[ruleID]; ok {
		if level := normalizeLevel(rule.Level); level != "" {
			return level
		}
	}
	switch ruleID {
	case "script_injection":
		return LevelBlock
	default:
		return LevelReview
	}
}

func textQualitySignal(text NormalizedComment, cfg appconfig.CommentModerationConfig) (Signal, bool) {
	runes := []rune(text.Compact)
	evidence := ""
	switch {
	case len(runes) == 0:
		evidence = "no_words"
	case len(runes) >= 5 && allDigits(runes):
		evidence = "numeric"
	case len(runes) >= 5 && mostlyNumberLike(runes):
		evidence = "number_like"
	case len(runes) >= 6 && mostlyRepeated(runes):
		evidence = "repeated"
	default:
		return Signal{}, false
	}
	level := structureRuleLevel("text_quality", cfg)
	return Signal{
		Source:   SourceStructure,
		Category: "text_quality",
		Level:    level,
		Score:    scoreForSignal(SourceStructure, "text_quality", evidence, level, cfg),
		Reason:   formatReason(SourceStructure, "text_quality", level, evidence),
		Evidence: evidence,
		RuleID:   "text_quality",
	}, true
}

func matchContactShapes(value string) []string {
	runes := []rune(value)
	for i, r := range runes {
		if !unicode.Is(unicode.So, r) {
			continue
		}
		next := skipFormatRunes(runes, i+1)
		next = skipSpaces(runes, next)
		if next >= len(runes) || (runes[next] != ':' && runes[next] != '：') {
			continue
		}
		accountStart := skipSpaces(runes, next+1)
		if hasAccountToken(runes[accountStart:]) {
			return []string{"symbol_account"}
		}
	}
	return nil
}

func skipFormatRunes(runes []rune, start int) int {
	for start < len(runes) && (runes[start] == '\ufe0f' || runes[start] == '\u200d') {
		start++
	}
	return start
}

func skipSpaces(runes []rune, start int) int {
	for start < len(runes) && unicode.IsSpace(runes[start]) {
		start++
	}
	return start
}

func hasAccountToken(runes []rune) bool {
	length := 0
	for _, r := range runes {
		if !(r == '_' || r == '-' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z')) {
			break
		}
		length++
	}
	return length >= 4
}

func allDigits(runes []rune) bool {
	for _, r := range runes {
		if !unicode.IsDigit(r) && !unicode.IsNumber(r) {
			return false
		}
	}
	return len(runes) > 0
}

func mostlyNumberLike(runes []rune) bool {
	count := 0
	for _, r := range runes {
		if unicode.IsDigit(r) || unicode.IsNumber(r) || strings.ContainsRune("零〇一二三四五六七八九壹贰叁肆伍陆柒捌玖两", r) {
			count++
		}
	}
	return float64(count)/float64(len(runes)) >= 0.6
}

func mostlyRepeated(runes []rune) bool {
	counts := make(map[rune]int, len(runes))
	maxCount := 0
	for _, r := range runes {
		counts[r]++
		if counts[r] > maxCount {
			maxCount = counts[r]
		}
	}
	return float64(maxCount)/float64(len(runes)) >= 0.75
}

func containsAnyCompact(views []string, values []string) []string {
	matches := make([]string, 0)
	seen := make(map[string]struct{})
	for _, view := range views {
		for _, value := range values {
			if value == "" || !strings.Contains(view, value) {
				continue
			}
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			matches = append(matches, value)
		}
	}
	return matches
}

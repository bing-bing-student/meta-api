package moderation

import (
	"regexp"
	"strings"
	"unicode"

	appconfig "meta-api/config"
)

var (
	scriptRegexp = regexp.MustCompile(`(?i)(<\s*script\b|javascript\s*:)`)
	phoneRegexp  = regexp.MustCompile(`\b1[3-9]\d{9}\b`)
)

// structureSignals 从脚本、URL、联系方式、风险短语、编码内容和文本质量中提取结构信号。
// 输入 text 是归一化评论，cfg 提供结构规则；返回值包含所有成立的结构风险证据。
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

// matchesRiskPhrase 判断 text 是否命中配置的风险短语或正则模式。
// 返回 true 表示至少命中一项，但不代表已经完成上下文决策。
func matchesRiskPhrase(text NormalizedComment, cfg appconfig.CommentModerationConfig) bool {
	return containsAnyNormalized(text.Compact, cfg.StructurePatterns.RiskPhrases) ||
		matchesAnySemanticPattern(text.Normalized, cfg.StructurePatterns.RiskPatterns)
}

// hasRiskyScriptClause 判断 text 是否存在不处于良性语境的脚本注入分句。
// 输入 cfg 用于分句和语境判断；返回 true 表示存在风险脚本结构。
func hasRiskyScriptClause(text NormalizedComment, cfg appconfig.CommentModerationConfig) bool {
	for _, clause := range semanticClauses(text, cfg) {
		if scriptRegexp.MatchString(clause.Normalized) && !isBenignSemanticClause(clause, cfg) {
			return true
		}
	}
	return false
}

// hasRiskyURLClause 判断 text 中的 URL 是否出现在可执行风险语境。
// 输入 cfg 提供 URL 模式及语境规则；返回 true 表示应产生 URL 结构信号。
func hasRiskyURLClause(text NormalizedComment, cfg appconfig.CommentModerationConfig) bool {
	if matchesConfiguredURL(text.Normalized, cfg) &&
		!allSemanticClausesBenign(text, cfg) {
		return true
	}
	for _, clause := range semanticClauses(text, cfg) {
		if !matchesConfiguredURL(clause.Normalized, cfg) {
			continue
		}
		if !isBenignSemanticClause(clause, cfg) {
			return true
		}
	}
	return false
}

// hasRiskyContactClause 判断 text 中的电话、账号或配置联系方式是否处于风险语境。
// 返回 true 表示至少一个分句满足联系方式形态且未被否定或良性上下文抑制。
func hasRiskyContactClause(text NormalizedComment, cfg appconfig.CommentModerationConfig) bool {
	fullMatch := phoneRegexp.MatchString(text.Normalized) ||
		matchesConfiguredContact(text.Normalized, cfg) ||
		len(matchContactShapes(text.Normalized, accountTokenMinimum(cfg))) > 0
	if fullMatch && !allSemanticClausesBenign(text, cfg) {
		return true
	}
	for _, clause := range semanticClauses(text, cfg) {
		value := clause.Normalized
		matched := phoneRegexp.MatchString(value) ||
			matchesConfiguredContact(value, cfg) ||
			len(matchContactShapes(value, accountTokenMinimum(cfg))) > 0
		if matched && !isNegatedContactMention(value, cfg) &&
			!isBenignContactMention(value, cfg) &&
			!isBenignSemanticClause(clause, cfg) {
			return true
		}
	}
	return false
}

// matchesConfiguredURL 使用 cfg 中的 URL 正则匹配 value；返回 true 表示至少命中一个模式。
func matchesConfiguredURL(value string, cfg appconfig.CommentModerationConfig) bool {
	return matchesAnySemanticPattern(value, cfg.StructurePatterns.URLPatterns)
}

// matchesConfiguredContact 使用 cfg 中的联系方式正则匹配 value；返回 true 表示至少命中一个模式。
func matchesConfiguredContact(value string, cfg appconfig.CommentModerationConfig) bool {
	return matchesAnySemanticPattern(value, cfg.StructurePatterns.ContactPatterns)
}

// allSemanticClausesBenign 判断 text 的所有有效分句是否均属于良性语境。
// 返回 true 仅表示所有分句均良性；无法得到分句时返回 false。
func allSemanticClausesBenign(text NormalizedComment, cfg appconfig.CommentModerationConfig) bool {
	clauses := semanticClauses(text, cfg)
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

// isNegatedContactMention 判断 value 是否同时包含联系方式及配置的否定联系方式标记。
// 返回 true 表示联系方式更可能是拒绝或禁止语境。
func isNegatedContactMention(value string, cfg appconfig.CommentModerationConfig) bool {
	return containsAnyNormalized(compactText(normalizeText(value)), cfg.StructurePatterns.NegatedContactMarkers) &&
		matchesConfiguredContact(value, cfg)
}

// isBenignContactMention 判断 value 是否命中配置的良性联系方式模式；返回 true 表示可作为抑制证据。
func isBenignContactMention(value string, cfg appconfig.CommentModerationConfig) bool {
	return matchesAnySemanticPattern(value, cfg.StructurePatterns.BenignContactPatterns)
}

// newStructureSignal 根据 ruleID 和 evidence 构造标准结构信号。
// 输入 cfg 用于解析等级和分值；返回值包含来源、分类、原因及规则标识。
func newStructureSignal(ruleID, evidence string, cfg appconfig.CommentModerationConfig) Signal {
	level := structureRuleLevel(ruleID, cfg)
	return Signal{
		Source:   SourceStructure,
		Category: ruleID,
		Level:    level,
		Score:    evidenceStrengthScore(SourceStructure, ruleID, ruleID, level, cfg),
		Reason:   formatReason(SourceStructure, ruleID, level, evidence),
		Evidence: evidence,
		RuleID:   ruleID,
	}
}

// structureRuleLevel 解析 ruleID 对应的结构规则等级。
// 输入 cfg 是审核配置；返回配置等级，缺失时脚本注入默认为拒绝，其他规则默认为待审核。
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

// textQualitySignal 根据空文本、纯数字、数字占比和重复字符判断文本质量。
// 输入 text 是归一化评论、cfg 提供阈值；返回信号及是否成立，正常文本返回零值和 false。
func textQualitySignal(text NormalizedComment, cfg appconfig.CommentModerationConfig) (Signal, bool) {
	runes := []rune(text.Compact)
	minNumeric, minRepeated, numberRatio, repeatedRatio := textQualityThresholds(cfg)
	evidence := ""
	switch {
	case len(runes) == 0:
		evidence = "no_words"
	case len(runes) >= minNumeric && allDigits(runes):
		evidence = "numeric"
	case len(runes) >= minNumeric && mostlyNumberLike(runes, numberRatio):
		evidence = "number_like"
	case len(runes) >= minRepeated && mostlyRepeated(runes, repeatedRatio):
		evidence = "repeated"
	default:
		return Signal{}, false
	}
	level := structureRuleLevel("text_quality", cfg)
	return Signal{
		Source:   SourceStructure,
		Category: "text_quality",
		Level:    level,
		Score:    evidenceStrengthScore(SourceStructure, "text_quality", evidence, level, cfg),
		Reason:   formatReason(SourceStructure, "text_quality", level, evidence),
		Evidence: evidence,
		RuleID:   "text_quality",
	}, true
}

// textQualityThresholds 从 cfg 读取文本质量阈值并补齐安全默认值。
// 返回值依次为数字最小长度、重复最小长度、数字占比阈值和重复占比阈值。
func textQualityThresholds(cfg appconfig.CommentModerationConfig) (int, int, float64, float64) {
	minNumeric := cfg.StructurePatterns.MinNumericRunes
	if minNumeric <= 0 {
		minNumeric = 5
	}
	minRepeated := cfg.StructurePatterns.MinRepeatedRunes
	if minRepeated <= 0 {
		minRepeated = 6
	}
	numberRatio := cfg.StructurePatterns.NumberLikeRatio
	if numberRatio <= 0 {
		numberRatio = 0.6
	}
	repeatedRatio := cfg.StructurePatterns.RepeatedRatio
	if repeatedRatio <= 0 {
		repeatedRatio = 0.75
	}
	return minNumeric, minRepeated, numberRatio, repeatedRatio
}

// accountTokenMinimum 读取联系方式账号的最小字符数；cfg 未配置有效值时返回默认值 4。
func accountTokenMinimum(cfg appconfig.CommentModerationConfig) int {
	if minimum := cfg.StructurePatterns.MinAccountTokenRunes; minimum > 0 {
		return minimum
	}
	return 4
}

// matchContactShapes 识别“符号:账号”形式的隐晦联系方式。
// 输入 value 是待检查文本、minAccountRunes 是账号最小长度；返回命中的结构标签，未命中时返回 nil。
func matchContactShapes(value string, minAccountRunes int) []string {
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
		if hasAccountToken(runes[accountStart:], minAccountRunes) {
			return []string{"symbol_account"}
		}
	}
	return nil
}

// skipFormatRunes 从 start 开始跳过变体选择符和零宽连接符。
// 输入 runes 是全文字符；返回第一个非格式字符的下标或切片长度。
func skipFormatRunes(runes []rune, start int) int {
	for start < len(runes) && (runes[start] == '\ufe0f' || runes[start] == '\u200d') {
		start++
	}
	return start
}

// skipSpaces 从 start 开始跳过 Unicode 空白；返回第一个非空白字符的下标或切片长度。
func skipSpaces(runes []rune, start int) int {
	for start < len(runes) && unicode.IsSpace(runes[start]) {
		start++
	}
	return start
}

// hasAccountToken 判断 runes 开头是否存在达到 minimum 长度的 ASCII 账号片段。
// 返回 true 表示连续字符只由小写字母、数字、下划线或连字符组成且长度达标。
func hasAccountToken(runes []rune, minimum int) bool {
	length := 0
	for _, r := range runes {
		if !(r == '_' || r == '-' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z')) {
			break
		}
		length++
	}
	return length >= minimum
}

// allDigits 判断 runes 是否为非空且全部属于 Unicode 数字；返回对应判断结果。
func allDigits(runes []rune) bool {
	for _, r := range runes {
		if !unicode.IsDigit(r) && !unicode.IsNumber(r) {
			return false
		}
	}
	return len(runes) > 0
}

// mostlyNumberLike 判断 runes 中数字及中文数字的占比是否达到 threshold；返回对应判断结果。
func mostlyNumberLike(runes []rune, threshold float64) bool {
	count := 0
	for _, r := range runes {
		if unicode.IsDigit(r) || unicode.IsNumber(r) || strings.ContainsRune("零〇一二三四五六七八九壹贰叁肆伍陆柒捌玖两", r) {
			count++
		}
	}
	return float64(count)/float64(len(runes)) >= threshold
}

// mostlyRepeated 判断 runes 中出现次数最多的字符占比是否达到 threshold；返回对应判断结果。
func mostlyRepeated(runes []rune, threshold float64) bool {
	counts := make(map[rune]int, len(runes))
	maxCount := 0
	for _, r := range runes {
		counts[r]++
		if counts[r] > maxCount {
			maxCount = counts[r]
		}
	}
	return float64(maxCount)/float64(len(runes)) >= threshold
}

// containsAnyCompact 汇总多个文本视图中命中的配置词项。
// 输入 views 是待检索文本、values 是候选词；返回按首次出现顺序去重后的命中项。
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

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
	accountRegexp          = regexp.MustCompile(`(?i)(加|群|qq|vx|v信|微信|筘|扣|抠|薇|威|扫码|二维码|企鹅群)[^\n]{0,12}\d[\d\s]{4,}|\bv\s*[:：]\s*[a-z0-9_-]{4,}`)
	contactIntentRegexp    = regexp.MustCompile(`(?i)(加\s*(微信|vx|v信|qq|群|筘|扣|抠|薇|威)|v我|联系\s*我|私信|进群|扫码|二维码|绿泡泡|绿信|微信号|账号写图片|联系方式)`)
	emailObfuscationRegexp = regexp.MustCompile(`(?i)\b[a-z0-9._%+-]+\s+at\s+[a-z0-9.-]+\s+dot\s+[a-z]{2,}\b`)
	riskPhraseRegexp       = regexp.MustCompile(`(?i)(政治敏感|打倒|灭亡|现有制度|统一发帖|集中刷屏|评论区集合|色情资源|成人交友|成人资料|特殊陪聊|同城特殊安排|激情聊天|留微信|微信号|威信|v信|vx\s*联系|广告|推广|返现|兼职赚钱|贷款|刷好评|刷屏推广|投注平台|赌博平台|赌博赚钱|稳赚|中奖|手续费|虚假投资|虚假理财|高收益|保本|骗子项目|彩票网站|虚拟币骗局|诈骗|黑客攻击|破解软件|激活码|揍到服|教训某人|教训那个人|现场集合|砸店|砸东西|砍人|威胁证人|报复社会|境外组织资料|月包服务|骗到钱|拜金炫富|普通人不配|极端享乐|嘲笑|嘲讽|活该|歧视|逃学赚钱|攀比借贷|低俗审丑|仇富仇贫)`)
)

func structureSignals(text NormalizedComment, cfg appconfig.CommentModerationConfig) []Signal {
	signals := make([]Signal, 0, 4)
	if scriptRegexp.MatchString(text.Normalized) {
		signals = append(signals, newStructureSignal("script_injection", "script", cfg))
	}
	if domainRegexp.MatchString(text.Normalized) {
		signals = append(signals, newStructureSignal("url", "url", cfg))
	}
	if phoneRegexp.MatchString(text.Normalized) || accountRegexp.MatchString(text.Normalized) ||
		contactIntentRegexp.MatchString(text.Normalized) ||
		emailObfuscationRegexp.MatchString(text.Normalized) ||
		len(matchContactShapes(text.Normalized)) > 0 {
		if !isNegatedContactMention(text.Normalized) {
			signals = append(signals, newStructureSignal("contact", "contact", cfg))
		}
	}
	if riskPhraseRegexp.MatchString(text.Normalized) {
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

func isNegatedContactMention(value string) bool {
	return strings.Contains(value, "不包含联系方式") ||
		strings.Contains(value, "没有联系方式") ||
		strings.Contains(value, "不留联系方式") ||
		strings.Contains(value, "无需联系方式")
}

func newStructureSignal(ruleID, evidence string, cfg appconfig.CommentModerationConfig) Signal {
	level := structureRuleLevel(ruleID, cfg)
	return Signal{
		Source:   SourceStructure,
		Category: ruleID,
		Level:    level,
		Score:    scoreForLevel(level, cfg),
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
		Score:    scoreForLevel(level, cfg),
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
			value = compactText(normalizeText(value))
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

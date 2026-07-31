package moderation

import (
	"strings"

	appconfig "meta-api/config"
)

func adjustSignalsBySemantics(text NormalizedComment, signals []Signal,
	cfg appconfig.CommentModerationConfig) []Signal {
	if len(signals) == 0 || cfg.SemanticRules.Disabled || hasNonSuppressibleSemanticRisk(signals) ||
		!hasBenignSemanticContext(text, cfg) {
		return signals
	}

	adjusted := make([]Signal, 0, len(signals))
	suppressed := make([]string, 0)
	for _, signal := range signals {
		if shouldSuppressByBenignContext(text, signal, cfg) {
			suppressed = append(suppressed, signal.Evidence)
			continue
		}
		adjusted = append(adjusted, signal)
	}
	if len(suppressed) == 0 {
		return signals
	}

	adjusted = append(adjusted, Signal{
		Source:   SourceSemantic,
		Category: "benign_context",
		Level:    LevelAllow,
		Evidence: strings.Join(compactSemanticEvidence(suppressed), ","),
		RuleID:   "benign_context",
	})
	return adjusted
}

func hasNonSuppressibleSemanticRisk(signals []Signal) bool {
	for _, signal := range signals {
		switch signal.Source {
		case SourceBehavior:
			return true
		case SourceStructure:
			switch signal.RuleID {
			case "url", "decoded_url", "script_injection":
				return true
			}
		}
	}
	return false
}

func shouldSuppressByBenignContext(text NormalizedComment, signal Signal, cfg appconfig.CommentModerationConfig) bool {
	if semanticValueAllowed(signal.Source, cfg.SemanticRules.BenignContext.SuppressSources) {
		return true
	}
	if signal.Source == SourceStructure &&
		semanticValueAllowed(signal.RuleID, cfg.SemanticRules.BenignContext.SuppressRuleIDs) {
		return true
	}
	if signal.Source == SourceLexicon &&
		semanticValueAllowed(signal.Category, cfg.SemanticRules.BenignContext.SuppressCategories) {
		return true
	}
	if hasExplicitModerationMetaContext(text) {
		switch signal.Source {
		case SourceContext:
			return true
		case SourceStructure:
			return signal.RuleID == "contact"
		}
	}
	return false
}

func semanticValueAllowed(value string, values []string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(values) == 0 {
		return false
	}
	for _, item := range values {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if item == "*" || item == value {
			return true
		}
	}
	return false
}

func hasBenignSemanticContext(text NormalizedComment, cfg appconfig.CommentModerationConfig) bool {
	markers := cfg.SemanticRules.BenignContext.Markers
	if len(markers) == 0 {
		markers = defaultBenignSemanticMarkers()
	}
	views := []string{text.Raw, text.Normalized, text.Compact, text.PinyinFolded}
	for _, view := range views {
		view = strings.TrimSpace(strings.ToLower(view))
		if view == "" {
			continue
		}
		for _, marker := range markers {
			marker = strings.TrimSpace(strings.ToLower(marker))
			if marker != "" && strings.Contains(view, marker) {
				return true
			}
		}
	}
	return false
}

func hasExplicitModerationMetaContext(text NormalizedComment) bool {
	markers := []string{
		"识别测试",
		"安全测试",
		"测试语料",
		"违规样例",
		"命中规则",
		"误杀",
		"误判",
		"不代表",
		"不是我的真实需求",
		"不是真实需求",
		"不要把",
		"正常讨论",
		"隐私保护案例",
		"风控系统",
		"审核策略",
		"审核系统",
		"内容安全系统",
		"举例说明",
		"案例说明",
		"只是引用",
		"自嘲",
	}
	return textContainsAnySemanticMarker(text, markers)
}

func textContainsAnySemanticMarker(text NormalizedComment, markers []string) bool {
	views := []string{text.Raw, text.Normalized, text.Compact, text.PinyinFolded}
	for _, view := range views {
		view = strings.TrimSpace(strings.ToLower(view))
		if view == "" {
			continue
		}
		for _, marker := range markers {
			marker = strings.TrimSpace(strings.ToLower(marker))
			if marker != "" && strings.Contains(view, marker) {
				return true
			}
		}
	}
	return false
}

func defaultBenignSemanticMarkers() []string {
	return []string{
		"什么叫",
		"有人说",
		"看到有人",
		"如果有人",
		"所谓",
		"这个词",
		"这种词",
		"这类词",
		"关键词",
		"话题",
		"案例",
		"例子",
		"样本",
		"测试",
		"测试数据",
		"讨论",
		"提到",
		"说法",
		"表达",
		"评价",
		"现象",
		"反诈骗",
		"没有刻意",
		"没有到",
		"反而",
		"如果一个",
		"即使",
		"说明原因",
		"环境差异",
		"初来乍到",
		"支持正版",
		"学术诚信",
		"行业乱象",
		"不代表",
		"并不代表",
		"不一定",
		"不应该",
		"不能",
		"不需要",
		"不要把",
		"不要因为",
		"不要只看",
		"无需",
		"没必要",
		"没有意义",
		"不适合",
		"更有帮助",
		"最好",
		"误杀",
		"误判",
		"审核系统",
		"风控系统",
		"内容安全系统",
		"命中规则",
		"违规样例",
		"不是我的真实需求",
		"自嘲",
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

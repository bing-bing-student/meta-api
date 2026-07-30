package comment

import (
	"encoding/json"
	"strings"
)

func encodeCommentModerationReasons(reasons []string) string {
	values := compactCommentModerationReasons(reasons)
	if len(values) == 0 {
		return ""
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return ""
	}
	return string(raw)
}

func decodeCommentModerationReasons(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}

	var reasons []string
	if err := json.Unmarshal([]byte(value), &reasons); err != nil {
		return []string{value}
	}
	return compactCommentModerationReasons(reasons)
}

func formatCommentModerationReasons(reasons []string) []string {
	reasons = compactCommentModerationReasons(reasons)
	if len(reasons) == 0 {
		return nil
	}
	values := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		values = append(values, formatCommentModerationReason(reason))
	}
	return values
}

func formatCommentModerationReason(reason string) string {
	parts := strings.Split(reason, ":")
	if len(parts) < 4 {
		return reason
	}

	source := moderationReasonSourceLabel(parts[0])
	category := moderationReasonCategoryLabel(parts[1])
	level := moderationReasonLevelLabel(parts[2])
	evidence := moderationReasonEvidenceLabel(parts[0], parts[len(parts)-1])

	if category == "" {
		return source + "：" + evidence + "（" + level + "）"
	}
	return source + "：" + evidence + "（" + category + "，" + level + "）"
}

func compactCommentModerationReasons(reasons []string) []string {
	if len(reasons) == 0 {
		return nil
	}
	values := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		reason = strings.TrimSpace(reason)
		if reason != "" {
			values = append(values, reason)
		}
	}
	return values
}

func moderationReasonSourceLabel(value string) string {
	switch value {
	case "lexicon":
		return "敏感词库命中"
	case "structure":
		return "结构规则命中"
	case "context":
		return "上下文规则命中"
	case "behavior":
		return "行为风险命中"
	default:
		return value
	}
}

func moderationReasonCategoryLabel(value string) string {
	switch value {
	case "sensitive":
		return "敏感内容"
	case "political":
		return "涉政风险"
	case "minor":
		return "未成年人风险"
	case "sexual":
		return "色情低俗"
	case "gore":
		return "血腥暴力"
	case "insult":
		return "辱骂攻击"
	case "harmful_value":
		return "不良价值观"
	case "spam_fraud":
		return "营销/诈骗"
	case "gambling":
		return "赌博风险"
	case "drugs":
		return "毒品风险"
	case "illegal_privacy":
		return "违法/隐私风险"
	case "contact":
		return "联系方式"
	case "url":
		return "链接"
	case "risk_phrase":
		return "风险话术"
	case "text_quality":
		return "文本质量异常"
	case "user_frequency":
		return "用户频率异常"
	case "ip_frequency":
		return "IP 频率异常"
	case "duplicate_content":
		return "重复内容"
	default:
		return value
	}
}

func moderationReasonLevelLabel(value string) string {
	switch value {
	case "review":
		return "待人工复核"
	case "block":
		return "建议拒绝"
	default:
		return value
	}
}

func moderationReasonEvidenceLabel(source, value string) string {
	switch value {
	case "contact":
		return "联系方式特征"
	case "url":
		return "链接特征"
	case "risk_phrase":
		return "风险话术"
	case "number_like":
		return "疑似联系方式数字串"
	case "user_frequency":
		return "同一用户短时间评论过多"
	case "ip_frequency":
		return "同一 IP 短时间评论过多"
	case "duplicate_content":
		return "重复提交相同内容"
	default:
		if source == "context" {
			return strings.ReplaceAll(value, "+", " + ")
		}
		return value
	}
}

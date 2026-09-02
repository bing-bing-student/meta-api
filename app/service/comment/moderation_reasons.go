package comment

import (
	"encoding/json"
	"strings"
)

// encodeCommentModerationReasons 清理 reasons 后编码为 JSON 数组字符串。
// 返回可写入数据库的文本；无有效原因或编码失败时返回空串。
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

// decodeCommentModerationReasons 将数据库字段 value 解码为审核原因列表。
// 返回清理后的原因；空值返回 nil，旧格式或非法 JSON 会作为单条原始原因保留。
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

// formatCommentModerationReasons 将机器可读 reasons 转换为面向管理员的中文说明。
// 返回保持输入顺序的格式化列表；无有效原因时返回 nil。
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

// formatCommentModerationReason 解析单条 reason 的来源、分类、等级和证据并生成中文描述。
// 输入不符合标准四段格式时原样返回，避免丢失未知或历史原因。
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

// compactCommentModerationReasons 去除 reasons 中的首尾空白和空项，返回保持原顺序的新切片。
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

// moderationReasonSourceLabel 将审核来源标识 value 转换为中文名称；未知来源原样返回。
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

// moderationReasonCategoryLabel 将审核分类标识 value 转换为中文名称；未知分类原样返回。
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

// moderationReasonLevelLabel 将审核等级 value 转换为面向管理员的中文处置建议；未知等级原样返回。
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

// moderationReasonEvidenceLabel 根据 source 将证据标识 value 转换为易读说明。
// 返回中文标签或原值；上下文组合证据会把加号展开为可读分隔。
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

package moderation

import (
	"encoding/base64"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

var urlShapeRegexp = regexp.MustCompile(`(?i)(https?\s*:\s*/\s*/|www\s*\.|[a-z0-9][a-z0-9._-]*\s*\.\s*(com|cn|net|org|top|xyz|shop|vip|cc|io|me)\b)`)

// Normalize 构建评论在各分析阶段使用的统一文本视图。
// 输入 raw 是未经处理的评论；返回值保留原文，并包含归一化、紧凑、形近骨架和可疑编码解码结果。
func Normalize(raw string) NormalizedComment {
	normalized := normalizeText(raw)
	compact := compactText(normalized)
	return NormalizedComment{
		Raw:          raw,
		Normalized:   normalized,
		Compact:      compact,
		Confusable:   confusableSkeleton(compact),
		DecodedTexts: decodedURLTexts(raw),
	}
}

// normalizeText 对 value 执行 Unicode、大小写、空白、控制字符、数字及高风险变体归一化。
// 返回值适合语义分析，但不会移除普通标点或符号。
func normalizeText(value string) string {
	value = norm.NFKC.String(strings.ToLower(value))
	var builder strings.Builder
	builder.Grow(len(value))
	lastSpace := false
	for _, r := range value {
		switch {
		case r == '\u200b' || r == '\u200c' || r == '\u200d' || r == '\ufeff' || r == '\ufe0f' || r == '\u20e3':
			continue
		case r == '\u3000':
			r = ' '
		case r >= '\uff01' && r <= '\uff5e':
			r -= '\ufee0'
		}
		r = normalizeStyledDigit(r)
		if unicode.IsControl(r) {
			continue
		}
		if unicode.IsSpace(r) {
			if !lastSpace {
				builder.WriteByte(' ')
				lastSpace = true
			}
			continue
		}
		builder.WriteRune(unicode.ToLower(r))
		lastSpace = false
	}
	normalized := normalizeRiskVariants(strings.TrimSpace(builder.String()))
	return normalizeChineseDigitsInNumericRuns(normalized)
}

// normalizeRiskVariants 将 value 中已知的繁体、错写及跨文字体系风险变体转换为统一形式。
// 返回值是替换后的文本；该步骤只处理无上下文歧义的等价变体。
func normalizeRiskVariants(value string) string {
	replacer := strings.NewReplacer(
		"價", "价",
		"岀", "出",
		"賣", "卖",
		"買", "买",
		"會員", "会员",
		"帳號", "账号",
		"賬號", "账号",
		"號", "号",
		"資源", "资源",
		"聯繫", "联系",
		"貸款", "贷款",
		"論文", "论文",
		"論", "论",
		"代寫", "代写",
		"寫", "写",
		"賭博", "赌博",
		"專區", "专区",
		"內部", "内部",
		"穩賺", "稳赚",
		"低伽", "低价",
		"出兽", "出售",
		"高級", "高级",
		"會圓", "会员",
		"帳戸", "账号",
		"薇訫", "微信",
		"作業", "作业",
		"保證", "保证",
		"詳情", "详情",
		"訫", "信",
		"х", "x",
		"а", "a",
		"о", "o",
	)
	return replacer.Replace(value)
}

// normalizeStyledDigit 将样式数字、圈号数字、上下标数字转换为 ASCII 数字。
// 输入 r 是单个字符；返回值是对应数字，无法转换时原样返回。
func normalizeStyledDigit(r rune) rune {
	switch {
	case r >= '0' && r <= '9':
		return r
	case r >= '０' && r <= '９':
		return r - '０' + '0'
	case r == '⓪' || r == '⓿':
		return '0'
	case r >= '①' && r <= '⑨':
		return r - '①' + '1'
	case r >= '⑴' && r <= '⑼':
		return r - '⑴' + '1'
	case r >= '⓵' && r <= '⓽':
		return r - '⓵' + '1'
	case r >= '❶' && r <= '❾':
		return r - '❶' + '1'
	case r >= '➀' && r <= '➈':
		return r - '➀' + '1'
	case r >= '➊' && r <= '➒':
		return r - '➊' + '1'
	case r >= '₀' && r <= '₉':
		return r - '₀' + '0'
	case r == '⁰':
		return '0'
	case r == '¹':
		return '1'
	case r == '²':
		return '2'
	case r == '³':
		return '3'
	case r >= '⁴' && r <= '⁹':
		return r - '⁴' + '4'
	default:
		return r
	}
}

// normalizeChineseDigitsInNumericRuns 将数字串中的中文数字按上下文转换为 ASCII 数字。
// 输入 value 是基础归一化文本；返回值仅在混合数字或连续中文数字场景执行转换，避免误伤普通词语。
func normalizeChineseDigitsInNumericRuns(value string) string {
	runes := []rune(value)
	var builder strings.Builder
	builder.Grow(len(value))

	for i := 0; i < len(runes); {
		if !isNumericRunRune(runes[i]) {
			builder.WriteRune(runes[i])
			i++
			continue
		}

		start := i
		hasASCII := false
		chineseDigits := 0
		for i < len(runes) && isNumericRunRune(runes[i]) {
			if runes[i] >= '0' && runes[i] <= '9' {
				hasASCII = true
			}
			if _, ok := chineseDigitValue(runes[i]); ok {
				chineseDigits++
			}
			i++
		}

		convertChinese := hasASCII || chineseDigits >= 2
		for _, r := range runes[start:i] {
			if convertChinese {
				if digit, ok := chineseDigitValue(r); ok {
					builder.WriteRune(digit)
					continue
				}
			}
			builder.WriteRune(r)
		}
	}

	return builder.String()
}

// isNumericRunRune 判断 r 是否可出现在待分析的数字片段中。
// 返回 true 表示 r 是数字、中文数字或用于分隔数字的空白及符号。
func isNumericRunRune(r rune) bool {
	if r >= '0' && r <= '9' {
		return true
	}
	if _, ok := chineseDigitValue(r); ok {
		return true
	}
	return unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r)
}

// chineseDigitValue 将中文数字字符 r 映射为 ASCII 数字。
// 返回值依次为映射字符和是否识别成功；未识别时返回零值与 false。
func chineseDigitValue(r rune) (rune, bool) {
	switch r {
	case '零', '〇':
		return '0', true
	case '一', '壹', '幺':
		return '1', true
	case '二', '贰', '两':
		return '2', true
	case '三', '叁':
		return '3', true
	case '四', '肆':
		return '4', true
	case '五', '伍':
		return '5', true
	case '六', '陆':
		return '6', true
	case '七', '柒':
		return '7', true
	case '八', '捌':
		return '8', true
	case '九', '玖':
		return '9', true
	default:
		return 0, false
	}
}

// compactText 从 value 中移除空白、标点和符号，生成用于抗拆词匹配的紧凑文本。
// 返回值保留字母、数字和文字的原有顺序。
func compactText(value string) string {
	var builder strings.Builder
	builder.Grow(len(value))
	for _, r := range value {
		if unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r) {
			continue
		}
		builder.WriteRune(r)
	}
	return builder.String()
}

// decodedURLTexts 从 value 中识别可解码为 URL 形态的 Base64 片段。
// 返回值是归一化、截断且去重后的解码证据；普通 Base64 内容不会返回。
func decodedURLTexts(value string) []string {
	candidates := extractBase64Candidates(value)
	matches := make([]string, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		decoded, ok := decodeBase64Candidate(candidate)
		if !ok || !urlShapeRegexp.MatchString(normalizeText(decoded)) {
			continue
		}
		match := formatDecodedURLMatch(decoded)
		if match == "" {
			continue
		}
		if _, ok := seen[match]; ok {
			continue
		}
		seen[match] = struct{}{}
		matches = append(matches, match)
	}
	return matches
}

// extractBase64Candidates 提取 value 中达到最小长度的连续 Base64 字符片段。
// 返回值只做字符形态筛选，不保证每个片段均可成功解码。
func extractBase64Candidates(value string) []string {
	candidates := make([]string, 0)
	start := -1
	for i := 0; i <= len(value); i++ {
		if i < len(value) && isBase64CandidateByte(value[i]) {
			if start < 0 {
				start = i
			}
			continue
		}
		if start >= 0 {
			if candidate := value[start:i]; len(candidate) >= base64MinLength {
				candidates = append(candidates, candidate)
			}
			start = -1
		}
	}
	return candidates
}

// isBase64CandidateByte 判断 value 是否属于标准或 URL 安全 Base64 字符集；返回 true 表示可纳入候选片段。
func isBase64CandidateByte(value byte) bool {
	return value == '+' || value == '/' || value == '=' || value == '-' || value == '_' ||
		(value >= '0' && value <= '9') ||
		(value >= 'A' && value <= 'Z') ||
		(value >= 'a' && value <= 'z')
}

// decodeBase64Candidate 尝试用标准及 URL 安全编码解码 value。
// 返回值依次为去除首尾空白的 UTF-8 文本和成功标记；格式非法或结果非 UTF-8 时返回 false。
func decodeBase64Candidate(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if len(value) < base64MinLength || strings.Count(value, "=") > 2 {
		return "", false
	}
	if index := strings.IndexByte(value, '='); index >= 0 && index < len(strings.TrimRight(value, "=")) {
		return "", false
	}

	raw := strings.TrimRight(value, "=")
	candidates := []struct {
		encoding *base64.Encoding
		value    string
	}{
		{encoding: base64.StdEncoding, value: value},
		{encoding: base64.StdEncoding, value: padBase64Candidate(raw)},
		{encoding: base64.RawStdEncoding, value: raw},
		{encoding: base64.URLEncoding, value: value},
		{encoding: base64.URLEncoding, value: padBase64Candidate(raw)},
		{encoding: base64.RawURLEncoding, value: raw},
	}
	for _, candidate := range candidates {
		if candidate.value == "" {
			continue
		}
		decoded, err := candidate.encoding.DecodeString(candidate.value)
		if err != nil || len(decoded) == 0 || !utf8.Valid(decoded) {
			continue
		}
		return strings.TrimSpace(string(decoded)), true
	}
	return "", false
}

// padBase64Candidate 按 Base64 长度规则为 value 补齐等号。
// 返回补齐后的字符串；长度余数为 1 时表示格式无效并返回空串。
func padBase64Candidate(value string) string {
	switch len(value) % 4 {
	case 0:
		return value
	case 2:
		return value + "=="
	case 3:
		return value + "="
	default:
		return ""
	}
}

// formatDecodedURLMatch 归一化并限制已解码 URL 证据的最大长度。
// 输入 value 是解码文本；返回值用于审核原因展示，空内容仍返回空串。
func formatDecodedURLMatch(value string) string {
	value = normalizeText(value)
	if value == "" {
		return ""
	}
	runes := []rune(value)
	if len(runes) > decodedURLReasonMaxLen {
		value = string(runes[:decodedURLReasonMaxLen])
	}
	return value
}

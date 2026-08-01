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

func Normalize(raw string) NormalizedComment {
	normalized := normalizeText(raw)
	compact := compactText(normalized)
	return NormalizedComment{
		Raw:          raw,
		Normalized:   normalized,
		Compact:      compact,
		Confusable:   confusableSkeleton(compact),
		PinyinFolded: foldMixedPinyin(compact),
		DecodedTexts: decodedURLTexts(raw),
	}
}

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
	normalized := normalizeRiskVariants(normalizeRiskEmoji(strings.TrimSpace(builder.String())))
	return normalizeChineseDigitsInNumericRuns(normalized)
}

func normalizeRiskEmoji(value string) string {
	replacer := strings.NewReplacer(
		"🕳", "坑",
		"💩", "屎",
		"🔨", "锤",
		"🐮", "牛",
		"🍺", "逼",
		"🈚", "无",
	)
	return replacer.Replace(value)
}

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

func isNumericRunRune(r rune) bool {
	if r >= '0' && r <= '9' {
		return true
	}
	if _, ok := chineseDigitValue(r); ok {
		return true
	}
	return unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r)
}

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

type pinyinFoldRule struct {
	pattern     string
	replacement string
}

var mixedPinyinFoldRules = []pinyinFoldRule{
	{pattern: "naoziyouwenti", replacement: "脑子有问题"},
	{pattern: "nizheshuiping", replacement: "你这水平"},
	{pattern: "zhuangdashen", replacement: "装大神"},
	{pattern: "budongzhuangdong", replacement: "不懂装懂"},
	{pattern: "pianliuliang", replacement: "骗流量"},
	{pattern: "wurenzidi", replacement: "误人子弟"},
	{pattern: "zhenexin", replacement: "真恶心"},
	{pattern: "nimade", replacement: "你妈的"},
	{pattern: "donggechuizi", replacement: "懂个锤子"},
	{pattern: "lianshixishengduburu", replacement: "连实习生都不如"},
	{pattern: "xiaoxuemeibiye", replacement: "小学没毕业"},
	{pattern: "dijidecuowu", replacement: "低级的错误"},
	{pattern: "zhuanmenchulaiwudaoxinren", replacement: "专门出来误导新人"},
	{pattern: "zhuanmenchulai", replacement: "专门出来"},
	{pattern: "wudaoxinren", replacement: "误导新人"},
	{pattern: "mianfeilingqu", replacement: "免费领取"},
	{pattern: "mianfeisong", replacement: "免费送"},
	{pattern: "ziyuanbao", replacement: "资源包"},
	{pattern: "neibujiaocheng", replacement: "内部教程"},
	{pattern: "dijiachushou", replacement: "低价出售"},
	{pattern: "chatgptzhanghao", replacement: "chatgpt账号"},
	{pattern: "claudezhanghao", replacement: "claude账号"},
	{pattern: "jiagebiguanfangdi", replacement: "价格比官方低"},
	{pattern: "pianyide", replacement: "便宜的"},
	{pattern: "fufeilunwendaixie", replacement: "付费论文代写"},
	{pattern: "lunwendaixie", replacement: "论文代写"},
	{pattern: "biyelunwen", replacement: "毕业论文"},
	{pattern: "biyesheji", replacement: "毕业设计"},
	{pattern: "xiangmudoukeyi", replacement: "项目都可以"},
	{pattern: "kaoshidaan", replacement: "考试答案"},
	{pattern: "youxidailiandaida", replacement: "游戏代练代打"},
	{pattern: "shuapaiming", replacement: "刷排名"},
	{pattern: "shuapinglun", replacement: "刷评论"},
	{pattern: "shualiangfuwu", replacement: "刷量服务"},
	{pattern: "kuaisushangremen", replacement: "快速上热门"},
	{pattern: "kuaisushangfen", replacement: "快速上分"},
	{pattern: "touziqun", replacement: "投资群"},
	{pattern: "neibuqudao", replacement: "内部渠道"},
	{pattern: "baozhengshouyi", replacement: "保证收益"},
	{pattern: "shenfenzhengzhaopian", replacement: "身份证照片"},
	{pattern: "kuaisubanlidaikuan", replacement: "快速办理贷款"},
	{pattern: "kuaisubanli", replacement: "快速办理"},
	{pattern: "chengrenwangzhanhuiyuan", replacement: "成人网站会员"},
	{pattern: "chengrenwangzhan", replacement: "成人网站"},
	{pattern: "gaoqingwumaziyuan", replacement: "高清无码资源"},
	{pattern: "gaoqingwuma", replacement: "高清无码"},
	{pattern: "wumaziyuan", replacement: "无码资源"},
	{pattern: "bocaizhanghao", replacement: "博彩账号"},
	{pattern: "duboxiazhu", replacement: "赌博下注"},
	{pattern: "xiazhujiqiao", replacement: "下注技巧"},
	{pattern: "wanquanziyuan", replacement: "完整资源"},
	{pattern: "jiahaoyou", replacement: "加好友"},
	{pattern: "zhanghao", replacement: "账号"},
	{pattern: "chushou", replacement: "出售"},
	{pattern: "daixie", replacement: "代写"},
	{pattern: "daizuo", replacement: "代做"},
	{pattern: "dailian", replacement: "代练"},
	{pattern: "daida", replacement: "代打"},
	{pattern: "shuafen", replacement: "刷分"},
	{pattern: "shualiang", replacement: "刷量"},
	{pattern: "shangfen", replacement: "上分"},
	{pattern: "daikuan", replacement: "贷款"},
	{pattern: "dubo", replacement: "赌博"},
	{pattern: "xiazhu", replacement: "下注"},
	{pattern: "wuma", replacement: "无码"},
	{pattern: "ziyuan", replacement: "资源"},
	{pattern: "jiaocheng", replacement: "教程"},
	{pattern: "mianfei", replacement: "免费"},
	{pattern: "liao", replacement: "聊"},
	{pattern: "luo", replacement: "裸"},
	{pattern: "yue", replacement: "约"},
	{pattern: "pao", replacement: "炮"},
	{pattern: "zhuang", replacement: "装"},
	{pattern: "qun", replacement: "群"},
	{pattern: "zi", replacement: "资"},
}

func foldMixedPinyin(value string) string {
	var builder strings.Builder
	builder.Grow(len(value))
	for i := 0; i < len(value); {
		if pattern, replacement, ok := matchMixedPinyinFold(value[i:]); ok {
			builder.WriteString(replacement)
			i += len(pattern)
			continue
		}
		r, size := utf8.DecodeRuneInString(value[i:])
		if r == utf8.RuneError && size == 0 {
			return foldRiskInitials(builder.String())
		}
		builder.WriteRune(r)
		i += size
	}
	return foldRiskInitials(builder.String())
}

func matchMixedPinyinFold(value string) (pattern, replacement string, ok bool) {
	for _, rule := range mixedPinyinFoldRules {
		if strings.HasPrefix(value, rule.pattern) {
			return rule.pattern, rule.replacement, true
		}
	}
	return "", "", false
}

func foldRiskInitials(value string) string {
	replacer := strings.NewReplacer(
		"d0g", "狗",
		"d写", "代写",
		"d设", "代设",
		"d做", "代做",
		"d练", "代练",
		"d打", "代打",
		"b业", "毕业",
		"刷f", "刷分",
		"上f", "上分",
		"刷p", "刷排名",
		"刷l", "刷量",
		"刷z", "刷赞",
		"投资q", "投资群",
		"内部qd", "内部渠道",
		"sfz", "身份证",
		"无m", "无码",
		"私m", "私密",
		"赌z", "赌资",
	)
	return replacer.Replace(value)
}

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

func isBase64CandidateByte(value byte) bool {
	return value == '+' || value == '/' || value == '=' || value == '-' || value == '_' ||
		(value >= '0' && value <= '9') ||
		(value >= 'A' && value <= 'Z') ||
		(value >= 'a' && value <= 'z')
}

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

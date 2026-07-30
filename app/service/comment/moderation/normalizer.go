package moderation

import (
	"encoding/base64"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var urlShapeRegexp = regexp.MustCompile(`(?i)(https?\s*:\s*/\s*/|www\s*\.|[a-z0-9][a-z0-9._-]*\s*\.\s*(com|cn|net|org|top|xyz|shop|vip|cc|io|me)\b)`)

func Normalize(raw string) NormalizedComment {
	normalized := normalizeText(raw)
	compact := compactText(normalized)
	return NormalizedComment{
		Raw:          raw,
		Normalized:   normalized,
		Compact:      compact,
		PinyinFolded: foldMixedPinyin(compact),
		DecodedTexts: decodedURLTexts(raw),
	}
}

func normalizeText(value string) string {
	value = strings.ToLower(value)
	var builder strings.Builder
	builder.Grow(len(value))
	lastSpace := false
	for _, r := range value {
		switch {
		case r == '\u200b' || r == '\u200c' || r == '\u200d' || r == '\ufeff':
			continue
		case r == '\u3000':
			r = ' '
		case r >= '\uff01' && r <= '\uff5e':
			r -= '\ufee0'
		}
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
	return strings.TrimSpace(builder.String())
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

func foldMixedPinyin(value string) string {
	var builder strings.Builder
	builder.Grow(len(value))
	for i := 0; i < len(value); {
		switch {
		case strings.HasPrefix(value[i:], "liao"):
			builder.WriteString("聊")
			i += len("liao")
		case strings.HasPrefix(value[i:], "luo"):
			builder.WriteString("裸")
			i += len("luo")
		case strings.HasPrefix(value[i:], "yue"):
			builder.WriteString("约")
			i += len("yue")
		case strings.HasPrefix(value[i:], "pao"):
			builder.WriteString("炮")
			i += len("pao")
		default:
			r, size := utf8.DecodeRuneInString(value[i:])
			if r == utf8.RuneError && size == 0 {
				return builder.String()
			}
			builder.WriteRune(r)
			i += size
		}
	}
	return builder.String()
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

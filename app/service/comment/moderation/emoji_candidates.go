package moderation

import (
	"context"
	"fmt"
	"strings"
)

const (
	maxEmojiOccurrences     = 4
	maxEmojiAnnotationTerms = 8
	maxEmojiInterpretations = 64
	maxEmojiRationaleRunes  = 80
)

// emojiVariantCandidates 将 clause 中 Emoji 的 CLDR 中文注释组合成解释文本，并与 riskIndex 匹配风险词。
// 输入 ctx 控制取消，clauseID 标识分句，maxCandidates 限制数量；返回本地、含歧义标记的改写候选。
func emojiVariantCandidates(ctx context.Context, clause NormalizedComment, clauseID int,
	riskIndex riskTermIndex, annotationIndex *emojiAnnotationIndex, maxCandidates int,
) []RewriteCandidate {
	if maxCandidates <= 0 || annotationIndex == nil {
		return nil
	}
	occurrences := annotationIndex.find(clause.Raw)
	if len(occurrences) == 0 {
		return nil
	}
	if len(occurrences) > maxEmojiOccurrences {
		occurrences = occurrences[:maxEmojiOccurrences]
	}
	interpretations := buildEmojiInterpretations(clause.Raw, occurrences)
	observed := emojiObservation(occurrences)
	result := make([]RewriteCandidate, 0, min(maxCandidates, 4))
	seen := make(map[string]struct{})
	appendCandidate := func(term riskTerm, interpretation, matched string, confidence float64) {
		if len(result) >= maxCandidates {
			return
		}
		key := term.Category + "\x00" + term.Role + "\x00" + term.Compact
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		result = append(result, RewriteCandidate{
			Text:       term.Text,
			Observed:   observed,
			Category:   term.Category,
			Role:       term.Role,
			Method:     "emoji_annotation",
			Confidence: confidence,
			Ambiguous:  true,
			Rationale: fmt.Sprintf("Unicode CLDR 本地标注候选：%s（匹配 %s）",
				truncateEmojiRationale(interpretation), matched),
			Clause: clauseID,
		})
	}

	for _, interpretation := range interpretations {
		if ctx.Err() != nil || len(result) >= maxCandidates {
			break
		}
		view := compactText(normalizeText(interpretation))
		if view == "" {
			continue
		}
		for _, length := range riskIndex.lengths {
			for _, term := range riskIndex.byLength[length] {
				if strings.Contains(view, term.Compact) {
					appendCandidate(term, interpretation, term.Compact, 0.92)
					continue
				}
				if term.RuneCount < 4 {
					continue
				}
				matched, distance := closestFuzzyWindow([]string{view}, term.Compact, 1)
				if matched != "" && distance == 1 {
					appendCandidate(term, interpretation, matched, 0.82)
				}
			}
		}
	}
	return result
}

// buildEmojiInterpretations 用每个 Emoji 的可用中文注释替换 value 中对应序列，生成受上限约束的组合解释。
// 输入 occurrences 必须按位置排列；返回包含非 Emoji 原文的候选解释集合。
func buildEmojiInterpretations(value string, occurrences []emojiOccurrence) []string {
	states := []string{""}
	cursor := 0
	for _, occurrence := range occurrences {
		literal := value[cursor:occurrence.Start]
		options := usableEmojiAnnotations(occurrence.Annotations)
		if len(options) == 0 {
			options = []string{""}
		}
		next := make([]string, 0, min(len(states)*len(options), maxEmojiInterpretations))
		seen := make(map[string]struct{})
		for _, state := range states {
			for _, option := range options {
				candidate := state + literal + option
				if _, exists := seen[candidate]; exists {
					continue
				}
				seen[candidate] = struct{}{}
				next = append(next, candidate)
				if len(next) >= maxEmojiInterpretations {
					break
				}
			}
			if len(next) >= maxEmojiInterpretations {
				break
			}
		}
		states = next
		cursor = occurrence.End
	}
	suffix := value[cursor:]
	for index := range states {
		states[index] += suffix
	}
	return states
}

// usableEmojiAnnotations 清理、去重并限制 values 中的 Emoji 中文注释。
// 返回最多 maxEmojiAnnotationTerms 个长度合适的紧凑词项。
func usableEmojiAnnotations(values []string) []string {
	result := make([]string, 0, min(len(values), maxEmojiAnnotationTerms))
	seen := make(map[string]struct{})
	for _, value := range values {
		value = compactText(normalizeText(value))
		if value == "" || len([]rune(value)) > 8 {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
		if len(result) >= maxEmojiAnnotationTerms {
			break
		}
	}
	return result
}

// emojiObservation 按出现顺序连接 occurrences 中的原始 Emoji，返回用于证据展示的观测字符串。
func emojiObservation(occurrences []emojiOccurrence) string {
	values := make([]string, 0, len(occurrences))
	for _, occurrence := range occurrences {
		values = append(values, occurrence.Text)
	}
	return strings.Join(values, "…")
}

// truncateEmojiRationale 将 value 限制为审核原因允许的最大字符数；超长时返回带省略号的截断结果。
func truncateEmojiRationale(value string) string {
	runes := []rune(value)
	if len(runes) <= maxEmojiRationaleRunes {
		return value
	}
	return string(runes[:maxEmojiRationaleRunes]) + "…"
}

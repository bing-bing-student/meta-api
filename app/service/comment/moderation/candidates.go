package moderation

import "strings"

const maxRewriteCandidates = 20

// rewriteCandidates exposes deterministic text interpretations without replacing
// the raw comment. Context-dependent interpretations are added by the local
// context analyzer instead of being committed here as global string mappings.
func rewriteCandidates(text NormalizedComment) []RewriteCandidate {
	candidates := make([]RewriteCandidate, 0, 5)
	seen := map[string]struct{}{strings.TrimSpace(text.Raw): {}}
	appendCandidate := func(candidate RewriteCandidate) {
		candidate.Text = strings.TrimSpace(candidate.Text)
		if candidate.Text == "" || len(candidates) >= maxRewriteCandidates {
			return
		}
		if _, ok := seen[candidate.Text]; ok {
			return
		}
		seen[candidate.Text] = struct{}{}
		candidate.Confidence = clampProbability(candidate.Confidence)
		candidates = append(candidates, candidate)
	}

	appendCandidate(RewriteCandidate{
		Text:       text.Normalized,
		Method:     "unicode_normalization",
		Confidence: 0.99,
		Rationale:  "Unicode、大小写、空白与已知等价字符归一化",
	})
	appendCandidate(RewriteCandidate{
		Text:       text.Compact,
		Method:     "separator_compaction",
		Confidence: 0.96,
		Rationale:  "移除可能用于拆词的空白、标点与符号",
	})
	appendCandidate(RewriteCandidate{
		Text:       text.Confusable,
		Method:     "confusable_skeleton",
		Confidence: 0.9,
		Ambiguous:  true,
		Rationale:  "跨文字体系的形近字符骨架",
	})
	for _, decoded := range text.DecodedTexts {
		appendCandidate(RewriteCandidate{
			Text:       decoded,
			Method:     "url_decode",
			Confidence: 0.98,
			Rationale:  "URL 编码内容解码",
		})
	}
	return candidates
}

func mergeRewriteCandidates(groups ...[]RewriteCandidate) []RewriteCandidate {
	merged := make([]RewriteCandidate, 0, maxRewriteCandidates)
	seen := make(map[string]struct{}, maxRewriteCandidates)
	for _, group := range groups {
		for _, candidate := range group {
			candidate.Text = strings.TrimSpace(candidate.Text)
			if candidate.Text == "" || len(merged) >= maxRewriteCandidates {
				continue
			}
			key := strings.ToLower(candidate.Method) + "\x00" + candidate.Observed + "\x00" +
				candidate.Category + "\x00" + candidate.Role + "\x00" + candidate.Text
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			candidate.Confidence = clampProbability(candidate.Confidence)
			merged = append(merged, candidate)
		}
	}
	return merged
}

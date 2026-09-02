package moderation

import "strings"

const maxRewriteCandidates = 20

// rewriteCandidates 根据 text 生成确定性的文本改写候选，但不覆盖原评论。
// 输入 text 是各阶段的归一化文本；返回值包含可供后续分析使用的候选文本、生成方式及置信度。
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

// mergeRewriteCandidates 合并多个候选分组并按“方法、观测值、分类、角色和文本”去重。
// 输入 groups 是不同分析器产生的候选集合；返回值最多包含 maxRewriteCandidates 个已清理候选。
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

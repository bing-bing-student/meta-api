package moderation

import (
	"fmt"
	"hash/fnv"
	"math/bits"
	"strings"

	appconfig "meta-api/config"
)

// confusableSkeleton 将 value 中跨文字体系的形近字符转换为统一骨架。
// 返回值用于相似匹配，不直接替换用户原文。
func confusableSkeleton(value string) string {
	var builder strings.Builder
	builder.Grow(len(value))
	for _, r := range value {
		builder.WriteRune(confusableRune(r))
	}
	return builder.String()
}

// confusableRune 将单个形近字符 r 映射到拉丁骨架字符；没有映射时原样返回。
func confusableRune(r rune) rune {
	switch r {
	case 'а', 'ɑ', 'α':
		return 'a'
	case 'в', 'β':
		return 'b'
	case 'с', 'ϲ':
		return 'c'
	case 'е', 'ε':
		return 'e'
	case 'һ':
		return 'h'
	case 'і', 'ι':
		return 'i'
	case 'ј':
		return 'j'
	case 'κ':
		return 'k'
	case 'м':
		return 'm'
	case 'ո':
		return 'n'
	case 'о', 'ο':
		return 'o'
	case 'р', 'ρ':
		return 'p'
	case 'ѕ':
		return 's'
	case 'т', 'τ':
		return 't'
	case 'υ':
		return 'u'
	case 'х', 'χ':
		return 'x'
	case 'у', 'γ':
		return 'y'
	default:
		return r
	}
}

// fuzzyLexiconSignals 在评论分句中对配置词表执行受限编辑距离匹配。
// 输入 text 是归一化评论，cfg 提供候选词及阈值；返回值是去重后的相似风险信号。
func fuzzyLexiconSignals(text NormalizedComment, cfg appconfig.CommentModerationConfig) []Signal {
	rule := cfg.Lexicon.Fuzzy
	if rule.Disabled || len(rule.CandidateWords) == 0 {
		return nil
	}
	maxDistance := rule.MaxDistance
	if maxDistance <= 0 {
		maxDistance = defaultFuzzyMaxDistance
	}
	minRunes := rule.MinWordRunes
	if minRunes <= 0 {
		minRunes = defaultFuzzyMinWordRunes
	}

	signals := make([]Signal, 0, 1)
	seen := make(map[string]struct{})
	for clauseIndex, clause := range semanticClauses(text, cfg) {
		views := []string{clause.Compact}
		if skeleton := confusableSkeleton(clause.Compact); skeleton != clause.Compact {
			views = append(views, skeleton)
		}
		for category, words := range rule.CandidateWords {
			for _, word := range words {
				candidate := word
				if len([]rune(candidate)) < minRunes {
					continue
				}
				matched, distance := closestFuzzyWindow(views, candidate, maxDistance)
				if matched == "" {
					continue
				}
				key := category + ":" + candidate
				if _, exists := seen[key]; exists {
					continue
				}
				seen[key] = struct{}{}
				evidence := fmt.Sprintf("%s~%s(d=%d)", matched, candidate, distance)
				signals = append(signals, Signal{
					Source:   SourceSimilarity,
					Category: category,
					Level:    LevelReview,
					Score:    evidenceStrengthScore(SourceSimilarity, category, "fuzzy_lexicon", LevelReview, cfg),
					Reason:   formatReason(SourceSimilarity, category, LevelReview, evidence),
					Evidence: evidence,
					RuleID:   "fuzzy_lexicon",
					Clause:   clauseIndex + 1,
				})
			}
		}
	}
	return signals
}

// closestFuzzyWindow 在多个文本视图中查找与 candidate 编辑成本最低的窗口。
// 输入 maxDistance 是允许的最大编辑距离；返回最佳窗口及折算距离，未达到阈值时返回空串和零。
func closestFuzzyWindow(views []string, candidate string, maxDistance int) (string, int) {
	target := []rune(candidate)
	maxCost := maxDistance * 2
	bestCost := maxCost + 1
	best := ""
	for _, view := range views {
		if strings.Contains(view, candidate) {
			continue
		}
		runes := []rune(view)
		minLength := len(target) - maxDistance
		if minLength < 1 {
			minLength = 1
		}
		maxLength := len(target) + maxDistance
		for start := 0; start < len(runes); start++ {
			for length := minLength; length <= maxLength && start+length <= len(runes); length++ {
				window := runes[start : start+length]
				cost := weightedEditDistance(window, target)
				if cost < bestCost {
					bestCost = cost
					best = string(window)
				}
			}
		}
	}
	if bestCost > maxCost {
		return "", 0
	}
	return best, (bestCost + 1) / 2
}

// weightedEditDistance 计算 left 与 right 的加权编辑成本，其中形近字符替换成本低于普通替换。
// 返回值使用二倍刻度表示成本，插入、删除和普通替换的成本均为 2。
func weightedEditDistance(left, right []rune) int {
	if len(left) == 0 {
		return len(right) * 2
	}
	previous := make([]int, len(right)+1)
	current := make([]int, len(right)+1)
	for index := range previous {
		previous[index] = index * 2
	}
	for i, leftRune := range left {
		current[0] = (i + 1) * 2
		for j, rightRune := range right {
			substitution := 2
			if leftRune == rightRune {
				substitution = 0
			} else if confusableRune(leftRune) == confusableRune(rightRune) {
				substitution = 1
			}
			current[j+1] = min(
				previous[j+1]+2,
				current[j]+2,
				previous[j]+substitution,
			)
		}
		previous, current = current, previous
	}
	return previous[len(right)]
}

// simHash 为 value 的字符二元组生成 64 位局部敏感指纹。
// 输入会先进行归一化和紧凑化；返回值用于近重复比较，空文本返回零。
func simHash(value string) uint64 {
	runes := []rune(compactText(normalizeText(value)))
	if len(runes) == 0 {
		return 0
	}
	weights := [64]int{}
	features := runeNGrams(runes, 2)
	for _, feature := range features {
		hash := fnv.New64a()
		_, _ = hash.Write([]byte(feature))
		sum := hash.Sum64()
		for bit := range 64 {
			if sum&(uint64(1)<<bit) != 0 {
				weights[bit]++
			} else {
				weights[bit]--
			}
		}
	}
	var result uint64
	for bit, weight := range weights {
		if weight >= 0 {
			result |= uint64(1) << bit
		}
	}
	return result
}

// runeNGrams 将 runes 按 size 生成连续字符片段。
// 返回全部 N 元组；字符数不超过 size 时返回整个输入组成的单个片段。
func runeNGrams(runes []rune, size int) []string {
	if len(runes) <= size {
		return []string{string(runes)}
	}
	result := make([]string, 0, len(runes)-size+1)
	for index := 0; index+size <= len(runes); index++ {
		result = append(result, string(runes[index:index+size]))
	}
	return result
}

// simHashDistance 计算两个 64 位 SimHash 指纹的汉明距离；返回不同位的数量。
func simHashDistance(left, right uint64) int {
	return bits.OnesCount64(left ^ right)
}

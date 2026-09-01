package moderation

import (
	"fmt"
	"hash/fnv"
	"math/bits"
	"strings"

	appconfig "meta-api/config"
)

func confusableSkeleton(value string) string {
	var builder strings.Builder
	builder.Grow(len(value))
	for _, r := range value {
		builder.WriteRune(confusableRune(r))
	}
	return builder.String()
}

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
	for clauseIndex, clause := range semanticClauses(text) {
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

func simHashDistance(left, right uint64) int {
	return bits.OnesCount64(left ^ right)
}

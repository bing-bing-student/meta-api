package moderation

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"

	appconfig "meta-api/config"
)

type policyCache struct {
	mu        sync.RWMutex
	signature string
	config    appconfig.CommentModerationConfig
}

func (c *policyCache) Resolve(cfg appconfig.CommentModerationConfig) (appconfig.CommentModerationConfig, error) {
	encoded, err := json.Marshal(cfg)
	if err != nil {
		return cfg, fmt.Errorf("encode moderation policy: %w", err)
	}
	signature := string(encoded)

	c.mu.RLock()
	if c.signature == signature {
		compiled := c.config
		c.mu.RUnlock()
		return compiled, nil
	}
	c.mu.RUnlock()

	if err := ValidateConfig(cfg); err != nil {
		return cfg, err
	}
	compiled := compileConfig(cfg)

	c.mu.Lock()
	c.signature = signature
	c.config = compiled
	c.mu.Unlock()
	return compiled, nil
}

// ValidateConfig rejects policy mistakes that could silently disable detection.
func ValidateConfig(cfg appconfig.CommentModerationConfig) error {
	fuzzy := cfg.Lexicon.Fuzzy
	if !fuzzy.Disabled && len(fuzzy.CandidateWords) > 0 {
		maxDistance := fuzzy.MaxDistance
		if maxDistance <= 0 {
			maxDistance = defaultFuzzyMaxDistance
		}
		if maxDistance > 2 {
			return fmt.Errorf("lexicon.fuzzy.max_distance must be <= 2")
		}
		minRunes := fuzzy.MinWordRunes
		if minRunes <= 0 {
			minRunes = defaultFuzzyMinWordRunes
		}
		for category, words := range fuzzy.CandidateWords {
			for _, word := range words {
				if len([]rune(compactText(normalizeText(word)))) < minRunes {
					return fmt.Errorf("lexicon.fuzzy.candidate_words[%s]: %q is shorter than min_word_runes",
						category, word)
				}
			}
		}
	}

	nearDuplicate := cfg.BehaviorRules.NearDuplicate
	if !nearDuplicate.Disabled {
		if nearDuplicate.MaxHammingDistance > 16 {
			return fmt.Errorf("behavior_rules.near_duplicate.max_hamming_distance must be <= 16")
		}
		if nearDuplicate.MaxLengthDifferencePercent > 50 {
			return fmt.Errorf("behavior_rules.near_duplicate.max_length_difference_percent must be <= 50")
		}
	}

	seen := make(map[string]struct{}, len(cfg.CombinationRules))
	for index, rule := range cfg.CombinationRules {
		id := strings.TrimSpace(rule.ID)
		if id == "" {
			return fmt.Errorf("combination_rules[%d]: id is required", index)
		}
		if _, exists := seen[id]; exists {
			return fmt.Errorf("combination_rules: duplicate id %q", id)
		}
		seen[id] = struct{}{}
		if len(compactPolicyTerms(rule.Subjects)) == 0 {
			return fmt.Errorf("combination_rules[%s]: subjects are required", id)
		}
		if len(compactPolicyTerms(rule.Predicates)) == 0 {
			return fmt.Errorf("combination_rules[%s]: predicates are required", id)
		}
		if level := strings.TrimSpace(rule.Level); level != "" && normalizeLevel(level) == "" {
			return fmt.Errorf("combination_rules[%s]: invalid level %q", id, rule.Level)
		}
	}

	if err := validatePatterns(
		"structure_patterns.risk_patterns",
		cfg.StructurePatterns.RiskPatterns,
	); err != nil {
		return err
	}
	return validatePatterns(
		"semantic_rules.contexts.actionable_patterns",
		cfg.SemanticRules.Contexts.ActionablePatterns,
	)
}

func validatePatterns(path string, patterns []string) error {
	for index, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			return fmt.Errorf("%s[%d]: pattern is empty", path, index)
		}
		if _, err := regexp.Compile(pattern); err != nil {
			return fmt.Errorf("%s[%d]: %w", path, index, err)
		}
	}
	return nil
}

func compileConfig(cfg appconfig.CommentModerationConfig) appconfig.CommentModerationConfig {
	cfg.StructurePatterns.RiskPhrases = compactPolicyTerms(cfg.StructurePatterns.RiskPhrases)
	for category, words := range cfg.Lexicon.Fuzzy.CandidateWords {
		compiled := compactPolicyTerms(words)
		for index := range compiled {
			compiled[index] = confusableSkeleton(compiled[index])
		}
		cfg.Lexicon.Fuzzy.CandidateWords[category] = compiled
	}
	for index := range cfg.CombinationRules {
		rule := &cfg.CombinationRules[index]
		rule.Subjects = compactPolicyTerms(rule.Subjects)
		rule.Predicates = compactPolicyTerms(rule.Predicates)
	}
	contexts := &cfg.SemanticRules.Contexts
	contexts.ReportingMarkers = compactPolicyTerms(contexts.ReportingMarkers)
	contexts.RejectionMarkers = compactPolicyTerms(contexts.RejectionMarkers)
	contexts.TechnicalMarkers = compactPolicyTerms(contexts.TechnicalMarkers)
	contexts.UnambiguousBenignMarkers = compactPolicyTerms(contexts.UnambiguousBenignMarkers)
	contexts.ActionableMarkers = compactPolicyTerms(contexts.ActionableMarkers)
	cfg.SemanticRules.AbusePolicy.SevereMarkers = compactPolicyTerms(
		cfg.SemanticRules.AbusePolicy.SevereMarkers,
	)
	return cfg
}

func compactPolicyTerms(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = compactText(normalizeText(value)); value != "" {
			result = append(result, value)
		}
	}
	return result
}

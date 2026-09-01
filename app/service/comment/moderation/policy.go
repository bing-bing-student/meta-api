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
	if err := validatePolicyRegistry(cfg); err != nil {
		return err
	}
	thresholds := cfg.DecisionEngine.Thresholds
	if thresholds.ApproveMax < 0 || thresholds.ApproveMax >= 1 {
		return fmt.Errorf("decision_engine.thresholds.approve_max must be in [0, 1)")
	}
	if thresholds.RejectMin < 0 || thresholds.RejectMin > 1 {
		return fmt.Errorf("decision_engine.thresholds.reject_min must be in [0, 1]")
	}
	if thresholds.ApproveMax > 0 && thresholds.RejectMin > 0 && thresholds.RejectMin <= thresholds.ApproveMax {
		return fmt.Errorf("decision_engine.thresholds.reject_min must be greater than approve_max")
	}
	if thresholds.MinConfidence < 0 || thresholds.MinConfidence > 1 {
		return fmt.Errorf("decision_engine.thresholds.min_confidence must be in [0, 1]")
	}
	if cfg.DecisionEngine.ContextAnalysis.MaxCandidates < 0 || cfg.DecisionEngine.ContextAnalysis.MaxCandidates > 100 {
		return fmt.Errorf("decision_engine.context_analysis.max_candidates must be in [0, 100]")
	}

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
		if err := validateConceptReferences(rule.ID, "subject_refs", rule.SubjectRefs, "subject", cfg.ConceptSets); err != nil {
			return err
		}
		if err := validateConceptReferences(rule.ID, "predicate_refs", rule.PredicateRefs, "predicate", cfg.ConceptSets); err != nil {
			return err
		}
		if len(compactPolicyTerms(expandConceptReferences(rule.Subjects, rule.SubjectRefs, cfg.ConceptSets))) == 0 {
			return fmt.Errorf("combination_rules[%s]: subjects are required", id)
		}
		if len(compactPolicyTerms(expandConceptReferences(rule.Predicates, rule.PredicateRefs, cfg.ConceptSets))) == 0 {
			return fmt.Errorf("combination_rules[%s]: predicates are required", id)
		}
		if len(cfg.Categories) > 0 {
			category := strings.ToLower(strings.TrimSpace(rule.Category))
			if category == "" {
				category = strings.ToLower(id)
			}
			if _, exists := cfg.Categories[category]; !exists {
				return fmt.Errorf("combination_rules[%s]: unknown category %q", id, category)
			}
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
	if err := validatePatterns("structure_patterns.url_patterns", cfg.StructurePatterns.URLPatterns); err != nil {
		return err
	}
	if err := validatePatterns("structure_patterns.contact_patterns", cfg.StructurePatterns.ContactPatterns); err != nil {
		return err
	}
	if err := validatePatterns("structure_patterns.benign_contact_patterns",
		cfg.StructurePatterns.BenignContactPatterns); err != nil {
		return err
	}
	for name, value := range map[string]float64{
		"number_like_ratio": cfg.StructurePatterns.NumberLikeRatio,
		"repeated_ratio":    cfg.StructurePatterns.RepeatedRatio,
	} {
		if value < 0 || value > 1 {
			return fmt.Errorf("structure_patterns.%s must be in [0, 1]", name)
		}
	}
	return validatePatterns(
		"semantic_rules.contexts.actionable_patterns",
		cfg.SemanticRules.Contexts.ActionablePatterns,
	)
}

func validatePolicyRegistry(cfg appconfig.CommentModerationConfig) error {
	if len(cfg.PolicyFiles) > 0 {
		if len(cfg.Categories) == 0 {
			return fmt.Errorf("categories are required when policy_files are configured")
		}
		vocabulary := cfg.SemanticRules.RelationVocabulary
		if len(vocabulary.NegationMarkers) == 0 || len(vocabulary.Actors) == 0 ||
			len(vocabulary.PersonTargets) == 0 {
			return fmt.Errorf("semantic_rules.relation_vocabulary requires negation_markers, actors and person_targets")
		}
		riskEvaluation := cfg.SemanticRules.RiskEvaluation
		if len(riskEvaluation.Outcomes) == 0 || len(riskEvaluation.JudgmentPredicates) == 0 ||
			len(riskEvaluation.QuestionMarkers) == 0 {
			return fmt.Errorf("semantic_rules.risk_evaluation requires outcomes, judgment_predicates and question_markers")
		}
		harmful := cfg.SemanticRules.HarmfulValuePolicy
		if !harmful.Disabled && (len(harmful.SelfPronouns) == 0 || len(harmful.OtherPronouns) == 0 ||
			len(harmful.AddressedTargets) == 0 || len(harmful.ReferenceSuffixes) == 0 ||
			len(harmful.OutcomeNegations) == 0) {
			return fmt.Errorf("semantic_rules.harmful_value_policy requires pronouns, targets, reference_suffixes and outcome_negations")
		}
		if len(cfg.DecisionEngine.Calibration.Sources) == 0 {
			return fmt.Errorf("decision_engine.calibration.sources are required")
		}
	}
	for id, category := range cfg.Categories {
		id = strings.ToLower(strings.TrimSpace(id))
		if id == "" {
			return fmt.Errorf("categories: empty category id")
		}
		if level := strings.TrimSpace(category.DefaultLevel); level != "" && normalizeLevel(level) == "" {
			return fmt.Errorf("categories[%s]: invalid default_level %q", id, category.DefaultLevel)
		}
		if len(cfg.PolicyFiles) > 0 && normalizeLevel(category.DefaultLevel) == "" {
			return fmt.Errorf("categories[%s]: default_level is required", id)
		}
	}
	if len(cfg.Categories) > 0 {
		if err := validateConfiguredCategories(cfg); err != nil {
			return err
		}
	}
	for index, outcome := range cfg.SemanticRules.RiskEvaluation.Outcomes {
		stance := strings.ToLower(strings.TrimSpace(outcome.Stance))
		if stance != RelationStanceWarning && stance != RelationStanceCondemnation {
			return fmt.Errorf("semantic_rules.risk_evaluation.outcomes[%d]: invalid stance %q", index, outcome.Stance)
		}
		if len(compactPolicyTerms(outcome.Roots)) == 0 {
			return fmt.Errorf("semantic_rules.risk_evaluation.outcomes[%d]: roots are required", index)
		}
	}
	for id, concept := range cfg.ConceptSets {
		id = strings.TrimSpace(id)
		if id == "" {
			return fmt.Errorf("concept_sets: empty concept set id")
		}
		role := strings.ToLower(strings.TrimSpace(concept.Role))
		if role != "" && role != "subject" && role != "predicate" && role != "common" {
			return fmt.Errorf("concept_sets[%s]: role must be subject, predicate or common", id)
		}
		if len(compactPolicyTerms(concept.Terms)) == 0 {
			return fmt.Errorf("concept_sets[%s]: terms are required", id)
		}
	}
	calibration := cfg.DecisionEngine.Calibration
	for name, value := range map[string]float64{
		"allow":                  calibration.Allow,
		"block":                  calibration.Block,
		"script_injection_block": calibration.ScriptInjectionBlock,
		"default":                calibration.Default,
	} {
		if value < 0 || value > 1 {
			return fmt.Errorf("decision_engine.calibration.%s must be in [0, 1]", name)
		}
	}
	for source, value := range calibration.Sources {
		if value < 0 || value > 1 {
			return fmt.Errorf("decision_engine.calibration.sources[%s] must be in [0, 1]", source)
		}
	}
	return nil
}

func validateConfiguredCategories(cfg appconfig.CommentModerationConfig) error {
	require := func(path, category string) error {
		category = strings.ToLower(strings.TrimSpace(category))
		if _, exists := cfg.Categories[category]; !exists {
			return fmt.Errorf("%s: unknown category %q", path, category)
		}
		return nil
	}
	for category := range cfg.Lexicon.CustomWords.Block {
		if err := require("lexicon.custom_words.block", category); err != nil {
			return err
		}
	}
	for category := range cfg.Lexicon.CustomWords.Review {
		if err := require("lexicon.custom_words.review", category); err != nil {
			return err
		}
	}
	for category := range cfg.Lexicon.Fuzzy.CandidateWords {
		if err := require("lexicon.fuzzy.candidate_words", category); err != nil {
			return err
		}
	}
	for category := range cfg.StructureRules {
		if err := require("structure_rules", category); err != nil {
			return err
		}
	}
	for category := range cfg.DecisionEngine.ContextAnalysis.RiskConcepts {
		if err := require("decision_engine.context_analysis.risk_concepts", category); err != nil {
			return err
		}
	}
	return nil
}

func validateConceptReferences(ruleID, field string, refs []string, expectedRole string,
	sets map[string]appconfig.CommentModerationConceptSetConfig,
) error {
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		set, exists := sets[ref]
		if !exists {
			return fmt.Errorf("combination_rules[%s].%s: unknown concept set %q", ruleID, field, ref)
		}
		role := strings.ToLower(strings.TrimSpace(set.Role))
		if role != "" && role != "common" && role != expectedRole {
			return fmt.Errorf("combination_rules[%s].%s: concept set %q has incompatible role %q",
				ruleID, field, ref, set.Role)
		}
	}
	return nil
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
	for id, concept := range cfg.ConceptSets {
		concept.Terms = compactPolicyTerms(concept.Terms)
		concept.Role = strings.ToLower(strings.TrimSpace(concept.Role))
		cfg.ConceptSets[id] = concept
	}
	cfg.StructurePatterns.RiskPhrases = compactPolicyTerms(cfg.StructurePatterns.RiskPhrases)
	cfg.StructurePatterns.NegatedContactMarkers = compactPolicyTerms(
		cfg.StructurePatterns.NegatedContactMarkers,
	)
	for category, words := range cfg.Lexicon.Fuzzy.CandidateWords {
		compiled := compactPolicyTerms(words)
		for index := range compiled {
			compiled[index] = confusableSkeleton(compiled[index])
		}
		cfg.Lexicon.Fuzzy.CandidateWords[category] = compiled
	}
	for index := range cfg.CombinationRules {
		rule := &cfg.CombinationRules[index]
		rule.SubjectRefs = compactStringValues(rule.SubjectRefs)
		rule.PredicateRefs = compactStringValues(rule.PredicateRefs)
		rule.Subjects = compactPolicyTerms(expandConceptReferences(rule.Subjects, rule.SubjectRefs, cfg.ConceptSets))
		rule.Predicates = compactPolicyTerms(expandConceptReferences(rule.Predicates, rule.PredicateRefs, cfg.ConceptSets))
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
	harmfulPolicy := &cfg.SemanticRules.HarmfulValuePolicy
	harmfulPolicy.SelfHarmActions = compactPolicyTerms(harmfulPolicy.SelfHarmActions)
	harmfulPolicy.DeathWishActions = compactPolicyTerms(harmfulPolicy.DeathWishActions)
	harmfulPolicy.DangerousActions = compactPolicyTerms(harmfulPolicy.DangerousActions)
	harmfulPolicy.DangerousSubstances = compactPolicyTerms(harmfulPolicy.DangerousSubstances)
	harmfulPolicy.IngestionActions = compactPolicyTerms(harmfulPolicy.IngestionActions)
	harmfulPolicy.IncitementMarkers = compactPolicyTerms(harmfulPolicy.IncitementMarkers)
	harmfulPolicy.IncitementSuffixes = compactPolicyTerms(harmfulPolicy.IncitementSuffixes)
	harmfulPolicy.IdeationMarkers = compactPolicyTerms(harmfulPolicy.IdeationMarkers)
	harmfulPolicy.PreventionMarkers = compactPolicyTerms(harmfulPolicy.PreventionMarkers)
	harmfulPolicy.EducationActors = compactPolicyTerms(harmfulPolicy.EducationActors)
	harmfulPolicy.EducationActions = compactPolicyTerms(harmfulPolicy.EducationActions)
	harmfulPolicy.CriticalOutcomes = compactPolicyTerms(harmfulPolicy.CriticalOutcomes)
	harmfulPolicy.SelfPronouns = compactPolicyTerms(harmfulPolicy.SelfPronouns)
	harmfulPolicy.OtherPronouns = compactPolicyTerms(harmfulPolicy.OtherPronouns)
	harmfulPolicy.AdditionalTargets = compactPolicyTerms(harmfulPolicy.AdditionalTargets)
	harmfulPolicy.AddressedTargets = compactPolicyTerms(harmfulPolicy.AddressedTargets)
	harmfulPolicy.ReferenceSuffixes = compactPolicyTerms(harmfulPolicy.ReferenceSuffixes)
	harmfulPolicy.OutcomeNegations = compactPolicyTerms(harmfulPolicy.OutcomeNegations)
	harmfulPolicy.PromotionConflicts = compactPolicyTerms(harmfulPolicy.PromotionConflicts)
	compileRelationVocabulary(&cfg.SemanticRules.RelationVocabulary)
	compileRiskEvaluationPolicy(&cfg.SemanticRules.RiskEvaluation)
	return cfg
}

func expandConceptReferences(values, refs []string,
	sets map[string]appconfig.CommentModerationConceptSetConfig,
) []string {
	result := append([]string(nil), values...)
	for _, ref := range refs {
		result = append(result, sets[strings.TrimSpace(ref)].Terms...)
	}
	return result
}

func compileRelationVocabulary(cfg *appconfig.CommentModerationRelationVocabularyConfig) {
	if cfg == nil {
		return
	}
	cfg.NegationMarkers = compactPolicyTerms(cfg.NegationMarkers)
	cfg.ImmediateNegationMarkers = compactPolicyTerms(cfg.ImmediateNegationMarkers)
	cfg.Actors = compactPolicyTerms(cfg.Actors)
	cfg.PersonTargets = compactPolicyTerms(cfg.PersonTargets)
	cfg.ContentTargets = compactPolicyTerms(cfg.ContentTargets)
	cfg.ResultConnectors = compactPolicyTerms(cfg.ResultConnectors)
	cfg.PromotionActions = compactPolicyTerms(cfg.PromotionActions)
	cfg.WeakReportingMarkers = compactPolicyTerms(cfg.WeakReportingMarkers)
	cfg.InterrogativePrefixes = compactPolicyTerms(cfg.InterrogativePrefixes)
	cfg.QuestionMarkers = compactPolicyTerms(cfg.QuestionMarkers)
	cfg.FirstPersonMarkers = compactPolicyTerms(cfg.FirstPersonMarkers)
	cfg.ContrastMarkers = compactPolicyTerms(cfg.ContrastMarkers)
}

func compileRiskEvaluationPolicy(cfg *appconfig.CommentModerationRiskEvaluationConfig) {
	if cfg == nil {
		return
	}
	for index := range cfg.Outcomes {
		cfg.Outcomes[index].Stance = strings.ToLower(strings.TrimSpace(cfg.Outcomes[index].Stance))
		cfg.Outcomes[index].Roots = compactPolicyTerms(cfg.Outcomes[index].Roots)
	}
	cfg.OutcomeSuffixes = compactPolicyTerms(cfg.OutcomeSuffixes)
	cfg.JudgmentPredicates = compactPolicyTerms(cfg.JudgmentPredicates)
	cfg.DemonstrativePredicates = compactPolicyTerms(cfg.DemonstrativePredicates)
	cfg.PostOutcomeRejections = compactPolicyTerms(cfg.PostOutcomeRejections)
	cfg.WarningPredicates = compactPolicyTerms(cfg.WarningPredicates)
	cfg.GovernanceActions = compactPolicyTerms(cfg.GovernanceActions)
	cfg.GovernanceModals = compactPolicyTerms(cfg.GovernanceModals)
	cfg.PromotionMarkers = compactPolicyTerms(cfg.PromotionMarkers)
	cfg.QuestionMarkers = compactPolicyTerms(cfg.QuestionMarkers)
	cfg.PromotionContrastMarkers = compactPolicyTerms(cfg.PromotionContrastMarkers)
	cfg.PromotionActionMarkers = compactPolicyTerms(cfg.PromotionActionMarkers)
}

func compactStringValues(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func compactPolicyTerms(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value = compactText(normalizeText(value)); value != "" {
			if _, exists := seen[value]; exists {
				continue
			}
			seen[value] = struct{}{}
			result = append(result, value)
		}
	}
	return result
}

package moderation

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"

	appconfig "meta-api/config"
)

// policyCache 缓存最近一份通过校验和编译的审核策略，并用签名避免重复编译。
type policyCache struct {
	mu        sync.RWMutex
	signature string
	config    appconfig.CommentModerationConfig
}

// Resolve 由 c 根据 cfg 签名复用或重新编译策略，返回可直接执行的配置和校验错误。
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

// ValidateConfig 校验 cfg 中可能静默关闭检测、破坏分类引用或产生无效阈值的策略错误。
// 返回首个配置错误；全部规则有效时返回 nil，函数不会修改输入配置。
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
	if err := validatePatterns("semantic_rules.contexts.actionable_patterns",
		cfg.SemanticRules.Contexts.ActionablePatterns); err != nil {
		return err
	}
	return validatePatterns("semantic_rules.relation_vocabulary.governance_patterns",
		cfg.SemanticRules.RelationVocabulary.GovernancePatterns)
}

// validatePolicyRegistry 检查 cfg 中的策略文件、分类、语义词组和校准来源是否完整，返回首个配置错误。
func validatePolicyRegistry(cfg appconfig.CommentModerationConfig) error {
	if len(cfg.PolicyFiles) > 0 && !cfg.Disabled {
		if len(cfg.Categories) == 0 {
			return fmt.Errorf("categories are required when policy_files are configured")
		}
		structure := cfg.StructurePatterns
		if err := validateRequiredPolicyTerms("structure_patterns",
			requiredPolicyTerms{"url_patterns", structure.URLPatterns},
			requiredPolicyTerms{"contact_patterns", structure.ContactPatterns},
			requiredPolicyTerms{"contact_labels", structure.ContactLabels},
			requiredPolicyTerms{"negated_contact_markers", structure.NegatedContactMarkers},
			requiredPolicyTerms{"benign_contact_patterns", structure.BenignContactPatterns},
		); err != nil {
			return err
		}
		contexts := cfg.SemanticRules.Contexts
		if err := validateRequiredPolicyTerms("semantic_rules.contexts",
			requiredPolicyTerms{"reporting_markers", contexts.ReportingMarkers},
			requiredPolicyTerms{"rejection_markers", contexts.RejectionMarkers},
			requiredPolicyTerms{"technical_markers", contexts.TechnicalMarkers},
			requiredPolicyTerms{"actionable_markers", contexts.ActionableMarkers},
		); err != nil {
			return err
		}
		vocabulary := cfg.SemanticRules.RelationVocabulary
		if err := validateRequiredPolicyTerms("semantic_rules.relation_vocabulary",
			requiredPolicyTerms{"negation_markers", vocabulary.NegationMarkers},
			requiredPolicyTerms{"immediate_negation_markers", vocabulary.ImmediateNegationMarkers},
			requiredPolicyTerms{"actors", vocabulary.Actors},
			requiredPolicyTerms{"person_targets", vocabulary.PersonTargets},
			requiredPolicyTerms{"content_targets", vocabulary.ContentTargets},
			requiredPolicyTerms{"result_connectors", vocabulary.ResultConnectors},
			requiredPolicyTerms{"promotion_actions", vocabulary.PromotionActions},
			requiredPolicyTerms{"weak_reporting_markers", vocabulary.WeakReportingMarkers},
			requiredPolicyTerms{"interrogative_prefixes", vocabulary.InterrogativePrefixes},
			requiredPolicyTerms{"question_markers", vocabulary.QuestionMarkers},
			requiredPolicyTerms{"first_person_markers", vocabulary.FirstPersonMarkers},
			requiredPolicyTerms{"contrast_markers", vocabulary.ContrastMarkers},
			requiredPolicyTerms{"clause_boundary_markers", vocabulary.ClauseBoundaryMarkers},
			requiredPolicyTerms{"governance_markers", vocabulary.GovernanceMarkers},
			requiredPolicyTerms{"governance_patterns", vocabulary.GovernancePatterns},
		); err != nil {
			return err
		}
		riskEvaluation := cfg.SemanticRules.RiskEvaluation
		if len(riskEvaluation.Outcomes) == 0 {
			return fmt.Errorf("semantic_rules.risk_evaluation.outcomes is required")
		}
		if err := validateRequiredPolicyTerms("semantic_rules.risk_evaluation",
			requiredPolicyTerms{"outcome_suffixes", riskEvaluation.OutcomeSuffixes},
			requiredPolicyTerms{"outcome_negations", riskEvaluation.OutcomeNegations},
			requiredPolicyTerms{"judgment_predicates", riskEvaluation.JudgmentPredicates},
			requiredPolicyTerms{"demonstrative_predicates", riskEvaluation.DemonstrativePredicates},
			requiredPolicyTerms{"post_outcome_rejections", riskEvaluation.PostOutcomeRejections},
			requiredPolicyTerms{"warning_predicates", riskEvaluation.WarningPredicates},
			requiredPolicyTerms{"governance_actions", riskEvaluation.GovernanceActions},
			requiredPolicyTerms{"governance_modals", riskEvaluation.GovernanceModals},
			requiredPolicyTerms{"promotion_markers", riskEvaluation.PromotionMarkers},
			requiredPolicyTerms{"question_markers", riskEvaluation.QuestionMarkers},
			requiredPolicyTerms{"promotion_contrast_markers", riskEvaluation.PromotionContrastMarkers},
			requiredPolicyTerms{"promotion_action_markers", riskEvaluation.PromotionActionMarkers},
		); err != nil {
			return err
		}
		harmful := cfg.SemanticRules.HarmfulValuePolicy
		if !harmful.Disabled {
			if err := validateRequiredPolicyTerms("semantic_rules.harmful_value_policy",
				requiredPolicyTerms{"self_harm_actions", harmful.SelfHarmActions},
				requiredPolicyTerms{"death_wish_actions", harmful.DeathWishActions},
				requiredPolicyTerms{"dangerous_actions", harmful.DangerousActions},
				requiredPolicyTerms{"dangerous_substances", harmful.DangerousSubstances},
				requiredPolicyTerms{"ingestion_actions", harmful.IngestionActions},
				requiredPolicyTerms{"incitement_markers", harmful.IncitementMarkers},
				requiredPolicyTerms{"incitement_suffixes", harmful.IncitementSuffixes},
				requiredPolicyTerms{"ideation_markers", harmful.IdeationMarkers},
				requiredPolicyTerms{"prevention_markers", harmful.PreventionMarkers},
				requiredPolicyTerms{"education_actors", harmful.EducationActors},
				requiredPolicyTerms{"education_actions", harmful.EducationActions},
				requiredPolicyTerms{"critical_outcomes", harmful.CriticalOutcomes},
				requiredPolicyTerms{"self_pronouns", harmful.SelfPronouns},
				requiredPolicyTerms{"other_pronouns", harmful.OtherPronouns},
				requiredPolicyTerms{"additional_targets", harmful.AdditionalTargets},
				requiredPolicyTerms{"addressed_targets", harmful.AddressedTargets},
				requiredPolicyTerms{"reference_suffixes", harmful.ReferenceSuffixes},
				requiredPolicyTerms{"outcome_negations", harmful.OutcomeNegations},
				requiredPolicyTerms{"promotion_conflicts", harmful.PromotionConflicts},
			); err != nil {
				return err
			}
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

// validateConfiguredCategories 校验 cfg 中词库、结构和上下文所引用的分类是否已注册，返回未知分类错误。
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

// validateConceptReferences 校验 ruleID 规则的 field 中 refs 是否存在且符合 expectedRole，返回首个引用错误。
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

// validatePatterns 逐个编译 patterns 中的正则，path 用于定位配置字段；返回空表达式或编译错误。
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

// requiredPolicyTerms 将必填策略字段名与其词项绑定，供统一完整性校验使用。
type requiredPolicyTerms struct {
	name   string
	values []string
}

// validateRequiredPolicyTerms 检查 scope 下的 groups 是否都含有非空词项，返回首个缺失字段错误。
func validateRequiredPolicyTerms(scope string, groups ...requiredPolicyTerms) error {
	for _, group := range groups {
		if len(compactPolicyTerms(group.values)) == 0 {
			return fmt.Errorf("%s.%s is required", scope, group.name)
		}
	}
	return nil
}

// compileConfig 对 cfg 执行词项归一化、概念引用展开和去重，返回不依赖原切片的可执行策略。
func compileConfig(cfg appconfig.CommentModerationConfig) appconfig.CommentModerationConfig {
	for id, concept := range cfg.ConceptSets {
		concept.Terms = compactPolicyTerms(concept.Terms)
		concept.Role = strings.ToLower(strings.TrimSpace(concept.Role))
		cfg.ConceptSets[id] = concept
	}
	cfg.StructurePatterns.RiskPhrases = compactPolicyTerms(cfg.StructurePatterns.RiskPhrases)
	cfg.StructurePatterns.ContactLabels = compactPolicyTerms(cfg.StructurePatterns.ContactLabels)
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

// expandConceptReferences 将 values 与 refs 在 sets 中指向的概念词合并，返回展开后的词项列表。
func expandConceptReferences(values, refs []string,
	sets map[string]appconfig.CommentModerationConceptSetConfig,
) []string {
	result := append([]string(nil), values...)
	for _, ref := range refs {
		result = append(result, sets[strings.TrimSpace(ref)].Terms...)
	}
	return result
}

// compileRelationVocabulary 对 cfg 中所有关系分析词组原地归一化和去重；无返回值。
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
	cfg.ClauseBoundaryMarkers = compactPolicyTerms(cfg.ClauseBoundaryMarkers)
	cfg.GovernanceMarkers = compactPolicyTerms(cfg.GovernanceMarkers)
}

// compileRiskEvaluationPolicy 对 cfg 中的风险评价立场和词组原地规范化；无返回值。
func compileRiskEvaluationPolicy(cfg *appconfig.CommentModerationRiskEvaluationConfig) {
	if cfg == nil {
		return
	}
	for index := range cfg.Outcomes {
		cfg.Outcomes[index].Stance = strings.ToLower(strings.TrimSpace(cfg.Outcomes[index].Stance))
		cfg.Outcomes[index].Roots = compactPolicyTerms(cfg.Outcomes[index].Roots)
	}
	cfg.OutcomeSuffixes = compactPolicyTerms(cfg.OutcomeSuffixes)
	cfg.OutcomeNegations = compactPolicyTerms(cfg.OutcomeNegations)
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

// compactStringValues 对 values 去除首尾空白、空值和重复项，返回保持首次出现顺序的列表。
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

// compactPolicyTerms 对 values 执行文本归一化、紧凑化和去重，返回可直接用于匹配的词项列表。
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

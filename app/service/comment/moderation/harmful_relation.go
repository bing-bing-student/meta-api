package moderation

import (
	"strings"

	appconfig "meta-api/config"
)

const (
	relationSubtypeSelfHarmEncouragement          = "self_harm_encouragement"
	relationSubtypeDangerousBehaviorEncouragement = "dangerous_behavior_encouragement"
	relationSubtypeDeathWish                      = "death_wish"
	relationSubtypeSelfHarmExpression             = "self_harm_expression"
	relationSubtypeRiskPrevention                 = "risk_prevention"
	relationSubtypeRiskEducation                  = "risk_education"
)

type harmfulRelationMatch struct {
	Canonical  string
	Observed   string
	Kind       string
	Auxiliary  string
	Confidence float64
}

func resolvedHarmfulValuePolicy(
	policy appconfig.CommentModerationHarmfulValuePolicyConfig,
) appconfig.CommentModerationHarmfulValuePolicyConfig {
	if len(policy.SelfPronouns) == 0 {
		policy.SelfPronouns = []string{"我们", "自己", "本人", "我"}
	}
	if len(policy.OtherPronouns) == 0 {
		policy.OtherPronouns = []string{"你们", "他们", "她们", "您", "你", "他", "她"}
	}
	if len(policy.AdditionalTargets) == 0 {
		policy.AdditionalTargets = []string{"同学", "孩子", "家人", "大家", "别人"}
	}
	if len(policy.AddressedTargets) == 0 {
		policy.AddressedTargets = []string{"你", "您", "你们", "同学", "大家"}
	}
	if len(policy.ReferenceSuffixes) == 0 {
		policy.ReferenceSuffixes = []string{"事件", "案例", "新闻", "原因", "现象", "知识", "问题", "话题", "含义"}
	}
	if len(policy.OutcomeNegations) == 0 {
		policy.OutcomeNegations = []string{"没有", "并无", "并不", "并非", "不算", "不能算", "不是", "不属于", "谈不上", "无"}
	}
	if len(policy.PromotionConflicts) == 0 {
		policy.PromotionConflicts = []string{"值得加入", "建议加入", "鼓励加入", "值得支持", "鼓励传播", "建议照做"}
	}
	return policy
}

func appendHarmfulValueRelations(clauseID int, clause NormalizedComment,
	candidates []RewriteCandidate, evidence []Evidence, cfg appconfig.CommentModerationConfig,
	appendRelation func(SemanticRelation),
) {
	policy := resolvedHarmfulValuePolicy(cfg.SemanticRules.HarmfulValuePolicy)
	if policy.Disabled {
		return
	}
	appendRiskEducationRelations(clauseID, clause, evidence, policy, cfg, appendRelation)

	for _, match := range harmfulRelationMatches(clauseID, clause, candidates, policy) {
		prevention := harmfulPreventionNear(clause.Compact, match.Observed, policy.PreventionMarkers)
		if prevention != "" {
			appendRelation(SemanticRelation{
				Clause: clauseID, Subject: "评论者", Action: "劝阻危险行为", Object: "相关人员",
				Result: matchResult(match), Category: "harmful_value", Subtype: relationSubtypeRiskPrevention,
				Evidence: prevention + "+" + matchEvidence(match), Negated: true, Confidence: 0.97,
			})
			continue
		}
		if harmfulConceptIsReferenced(clause.Compact, match.Observed, policy) {
			continue
		}

		if isSelfHarmExpression(clause.Compact, match, policy) {
			appendRelation(SemanticRelation{
				Clause: clauseID, Subject: "评论者", Action: "表达自伤意图", Object: "评论者",
				Result: matchResult(match), Category: "harmful_value", Subtype: relationSubtypeSelfHarmExpression,
				Evidence: matchEvidence(match), Confidence: minProbability(match.Confidence, 0.62),
			})
			continue
		}

		target := harmfulRelationTarget(clause.Compact, match.Observed, policy, cfg)
		directive := harmfulIncitementNear(clause.Compact, match.Observed, policy)
		if match.Kind == relationSubtypeDeathWish && target != "" && directive == "" {
			directive = "直接对对象表达"
		}
		if target == "" || directive == "" ||
			!harmfulTargetIsAddressed(clause.Compact, target, match.Observed, directive, policy) {
			continue
		}

		relation := SemanticRelation{
			Clause: clauseID, Subject: "评论者", Object: target, Result: matchResult(match),
			Category: "harmful_value", Subtype: match.Kind,
			Evidence: target + "+" + directive + "+" + matchEvidence(match), Confidence: match.Confidence,
			Quoted:   relationTermsQuoted(clause.Raw, match.Observed),
			Reported: relationReportedNear(clause.Normalized, match.Observed, cfg),
		}
		switch match.Kind {
		case relationSubtypeDeathWish:
			relation.Action = "死亡诅咒"
		case relationSubtypeDangerousBehaviorEncouragement:
			relation.Action = "教唆危险行为"
		default:
			relation.Action = "诱导自伤"
		}
		appendRelation(relation)
	}
}

func harmfulRelationMatches(clauseID int, clause NormalizedComment, candidates []RewriteCandidate,
	policy appconfig.CommentModerationHarmfulValuePolicyConfig,
) []harmfulRelationMatch {
	result := make([]harmfulRelationMatch, 0, 4)
	seen := make(map[string]struct{})
	appendMatch := func(canonical, observed, kind string, confidence float64) {
		canonical = compactText(normalizeText(canonical))
		observed = compactText(normalizeText(observed))
		if canonical == "" || observed == "" {
			return
		}
		key := kind + "\x00" + canonical + "\x00" + observed
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		result = append(result, harmfulRelationMatch{
			Canonical: canonical, Observed: observed, Kind: kind, Confidence: confidence,
		})
	}
	appendDirect := func(terms []string, kind string) {
		for _, term := range containedRelationTerms(clause.Compact, terms) {
			appendMatch(term, term, kind, 0.98)
		}
	}
	appendDirect(policy.SelfHarmActions, relationSubtypeSelfHarmEncouragement)
	appendDirect(policy.DeathWishActions, relationSubtypeDeathWish)
	appendDirect(policy.DangerousActions, relationSubtypeDangerousBehaviorEncouragement)
	appendDirect(policy.DangerousSubstances, relationSubtypeDangerousBehaviorEncouragement)

	for _, candidate := range candidates {
		if candidate.Clause != clauseID || !strings.EqualFold(candidate.Category, "harmful_value") ||
			candidate.Role != CandidateRoleConcept {
			continue
		}
		canonical := compactText(normalizeText(candidate.Text))
		kind := ""
		switch {
		case policyContains(policy.SelfHarmActions, canonical):
			kind = relationSubtypeSelfHarmEncouragement
		case policyContains(policy.DeathWishActions, canonical):
			kind = relationSubtypeDeathWish
		case policyContains(policy.DangerousActions, canonical),
			policyContains(policy.DangerousSubstances, canonical):
			kind = relationSubtypeDangerousBehaviorEncouragement
		}
		if kind != "" {
			appendMatch(canonical, candidate.Observed, kind, minProbability(candidate.Confidence, 0.96))
		}
	}

	filtered := result[:0]
	for _, match := range result {
		if policyContains(policy.DangerousSubstances, match.Canonical) {
			ingestion := harmfulIngestionNear(clause.Compact, match.Observed, policy.IngestionActions)
			if ingestion == "" {
				continue
			}
			match.Auxiliary = ingestion
		}
		filtered = append(filtered, match)
	}
	return filtered
}

func appendRiskEducationRelations(clauseID int, clause NormalizedComment, evidence []Evidence,
	policy appconfig.CommentModerationHarmfulValuePolicyConfig, cfg appconfig.CommentModerationConfig,
	appendRelation func(SemanticRelation),
) {
	actor, action, outcome, ok := riskEducationComponents(clause, policy, cfg)
	if !ok {
		return
	}

	bestObjectByCategory := make(map[string]string)
	for _, item := range evidence {
		if item.Polarity != "positive" || item.Category == "" || item.Category == "benign_context" ||
			item.Clause > 0 && item.Clause != clauseID {
			continue
		}
		object := evidenceObjectInClause(clause.Compact, item.Value)
		if object == "" {
			continue
		}
		if current := bestObjectByCategory[item.Category]; len([]rune(object)) > len([]rune(current)) {
			bestObjectByCategory[item.Category] = object
		}
	}
	for category, object := range bestObjectByCategory {
		appendRelation(SemanticRelation{
			Clause: clauseID, Subject: actor, Action: action, Object: object, Result: outcome,
			Category: category, Subtype: relationSubtypeRiskEducation,
			Evidence: actor + "+" + action + "+" + object + "+" + outcome,
			Reported: true, Confidence: 0.97,
		})
	}
}

func isRiskEducationSemanticClause(clause NormalizedComment, cfg appconfig.CommentModerationConfig) bool {
	if cfg.SemanticRules.HarmfulValuePolicy.Disabled {
		return false
	}
	_, _, _, ok := riskEducationComponents(clause,
		resolvedHarmfulValuePolicy(cfg.SemanticRules.HarmfulValuePolicy), cfg)
	return ok
}

func riskEducationComponents(clause NormalizedComment,
	policy appconfig.CommentModerationHarmfulValuePolicyConfig, cfg appconfig.CommentModerationConfig,
) (string, string, string, bool) {
	actor := firstContainedRelationTerm(clause.Compact, policy.EducationActors)
	action := firstContainedRelationTerm(clause.Compact, policy.EducationActions)
	outcome := firstContainedRelationTerm(clause.Compact, policy.CriticalOutcomes)
	ok := actor != "" && action != "" && outcome != "" &&
		!criticalOutcomeNegated(clause.Compact, outcome, policy.OutcomeNegations) &&
		!educationHasPromotionalConflict(clause.Compact, cfg, policy)
	return actor, action, outcome, ok
}

func policyContains(terms []string, canonical string) bool {
	canonical = compactText(normalizeText(canonical))
	for _, term := range terms {
		if compactText(normalizeText(term)) == canonical {
			return true
		}
	}
	return false
}

func harmfulIngestionNear(value, focus string, actions []string) string {
	prefix, _ := relationWindows(value, focus, 5, 2)
	best := ""
	for _, action := range actions {
		action = compactText(normalizeText(action))
		if action != "" && strings.Contains(prefix, action) && len([]rune(action)) > len([]rune(best)) {
			best = action
		}
	}
	return best
}

func harmfulPreventionNear(value, focus string, markers []string) string {
	prefix, suffix := relationWindows(value, focus, 12, 10)
	for _, marker := range markers {
		marker = compactText(normalizeText(marker))
		if marker != "" && (strings.Contains(prefix, marker) || strings.Contains(suffix, marker)) {
			return marker
		}
	}
	return ""
}

func harmfulIncitementNear(value, focus string,
	policy appconfig.CommentModerationHarmfulValuePolicyConfig,
) string {
	prefix, suffix := relationWindows(value, focus, 12, 6)
	for _, marker := range policy.IncitementMarkers {
		marker = compactText(normalizeText(marker))
		if marker != "" && strings.Contains(prefix, marker) {
			return marker
		}
	}
	for _, marker := range policy.IncitementSuffixes {
		marker = compactText(normalizeText(marker))
		if marker != "" && strings.Contains(suffix, marker) {
			return marker
		}
	}
	return ""
}

func relationWindows(value, focus string, before, after int) (string, string) {
	value = compactText(normalizeText(value))
	focus = compactText(normalizeText(focus))
	index := strings.Index(value, focus)
	if focus == "" || index < 0 {
		return value, value
	}
	prefixRunes := []rune(value[:index])
	if len(prefixRunes) > before {
		prefixRunes = prefixRunes[len(prefixRunes)-before:]
	}
	suffixRunes := []rune(value[index+len(focus):])
	if len(suffixRunes) > after {
		suffixRunes = suffixRunes[:after]
	}
	return string(prefixRunes), string(suffixRunes)
}

func isSelfHarmExpression(value string, match harmfulRelationMatch,
	policy appconfig.CommentModerationHarmfulValuePolicyConfig,
) bool {
	if match.Kind != relationSubtypeSelfHarmEncouragement {
		return false
	}
	prefix, _ := relationWindows(value, match.Observed, 10, 2)
	if !containsAnyNormalized(prefix, policy.IdeationMarkers) {
		return false
	}
	return nearestRelationPerson(prefix, policy) == "self"
}

func nearestRelationPerson(value string, policy appconfig.CommentModerationHarmfulValuePolicyConfig) string {
	lastIndex := -1
	role := ""
	for _, item := range []struct {
		role  string
		terms []string
	}{
		{"self", policy.SelfPronouns},
		{"other", policy.OtherPronouns},
	} {
		for _, term := range item.terms {
			if index := strings.LastIndex(value, term); index > lastIndex {
				lastIndex = index
				role = item.role
			}
		}
	}
	return role
}

func harmfulRelationTarget(value, focus string, policy appconfig.CommentModerationHarmfulValuePolicyConfig,
	cfg appconfig.CommentModerationConfig,
) string {
	prefix, _ := relationWindows(value, focus, 18, 0)
	targets := append([]string(nil), resolvedRelationVocabulary(cfg).PersonTargets...)
	targets = append(targets, policy.AdditionalTargets...)
	return firstContainedRelationTerm(prefix, targets)
}

func harmfulTargetIsAddressed(value, target, focus, directive string,
	policy appconfig.CommentModerationHarmfulValuePolicyConfig,
) bool {
	if containsAnyNormalized(target, policy.AddressedTargets) {
		return true
	}
	prefix, _ := relationWindows(value, focus, 18, 0)
	return containsAnyNormalized(prefix, []string{
		"让" + target, "叫" + target, "逼" + target, target + "有本事就",
		target + directive, directive + target,
	})
}

func harmfulConceptIsReferenced(value, focus string,
	policy appconfig.CommentModerationHarmfulValuePolicyConfig,
) bool {
	_, suffix := relationWindows(value, focus, 0, 5)
	for _, marker := range policy.ReferenceSuffixes {
		if strings.HasPrefix(suffix, marker) {
			return true
		}
	}
	return false
}

func criticalOutcomeNegated(value, outcome string, markers []string) bool {
	prefix, _ := relationWindows(value, outcome, 6, 0)
	return containsAnyNormalized(prefix, markers)
}

func educationHasPromotionalConflict(value string, cfg appconfig.CommentModerationConfig,
	policy appconfig.CommentModerationHarmfulValuePolicyConfig,
) bool {
	if containsAnyNormalized(value, cfg.SemanticRules.Contexts.ActionableMarkers) ||
		matchesAnySemanticPattern(value, cfg.SemanticRules.Contexts.ActionablePatterns) {
		return true
	}
	return containsAnyNormalized(value, policy.PromotionConflicts)
}

func evidenceObjectInClause(clause, value string) string {
	best := ""
	for _, term := range evidenceTerms(value) {
		if strings.Contains(clause, term) && len([]rune(term)) > len([]rune(best)) {
			best = term
		}
	}
	return best
}

func matchResult(match harmfulRelationMatch) string {
	if match.Auxiliary != "" {
		return match.Auxiliary + match.Canonical
	}
	return match.Canonical
}

func matchEvidence(match harmfulRelationMatch) string {
	result := matchResult(match)
	if match.Observed != match.Canonical {
		return match.Observed + "→" + result
	}
	return result
}

func minProbability(left, right float64) float64 {
	if left < right {
		return left
	}
	return right
}

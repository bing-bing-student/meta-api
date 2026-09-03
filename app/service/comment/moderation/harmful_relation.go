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

// harmfulRelationMatch 表示分句中识别到的有害行为概念、观测形式、子类型和辅助动作。
type harmfulRelationMatch struct {
	Canonical  string
	Observed   string
	Kind       string
	Auxiliary  string
	Confidence float64
}

// appendHarmfulValueRelations 从单个分句提取自伤诱导、危险行为、死亡诅咒、风险防范和科普关系。
// 输入 clauseID 标识分句，candidates 和 evidence 提供还原及证据，cfg 提供策略；结果通过 appendRelation 回调输出。
func appendHarmfulValueRelations(clauseID int, clause NormalizedComment,
	candidates []RewriteCandidate, evidence []Evidence, cfg appconfig.CommentModerationConfig,
	appendRelation func(SemanticRelation),
) {
	policy := cfg.SemanticRules.HarmfulValuePolicy
	if policy.Disabled {
		return
	}
	appendRiskEducationRelations(clauseID, clause, evidence, policy, cfg, appendRelation)

	for _, match := range harmfulRelationMatches(clauseID, clause, candidates, policy) {
		prevention := harmfulPreventionNear(clause.Compact, match.Observed, policy)
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

// harmfulRelationMatches 汇总 clause 中直接命中及候选还原得到的有害概念。
// 输入 clauseID 用于筛选 candidates，policy 提供概念词和摄入动作；返回去重且通过必要动作约束的匹配。
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

// appendRiskEducationRelations 将“主体科普风险并说明危害”的结构转换为对应分类的反向语义关系。
// 输入 evidence 用于定位被科普对象，policy 和 cfg 提供语法词项；关系通过 appendRelation 回调输出，无返回值。
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

// isRiskEducationSemanticClause 判断 clause 是否满足启用状态下的风险科普语法；返回对应结果。
func isRiskEducationSemanticClause(clause NormalizedComment, cfg appconfig.CommentModerationConfig) bool {
	if cfg.SemanticRules.HarmfulValuePolicy.Disabled {
		return false
	}
	_, _, _, ok := riskEducationComponents(clause, cfg.SemanticRules.HarmfulValuePolicy, cfg)
	return ok
}

// riskEducationComponents 从 clause 中提取科普主体、动作和危害结果。
// 输入 policy 和 cfg 提供词项及冲突规则；返回三个组成部分和结构是否完整且无否定、推广冲突。
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

// policyContains 判断规范化后的 canonical 是否与 terms 中任一策略词完全相等；返回对应结果。
func policyContains(terms []string, canonical string) bool {
	canonical = compactText(normalizeText(canonical))
	for _, term := range terms {
		if compactText(normalizeText(term)) == canonical {
			return true
		}
	}
	return false
}

// harmfulIngestionNear 在 focus 之前的局部窗口中查找最长摄入动作。
// 输入 value 是分句、actions 是动作词；返回匹配动作，未命中时返回空串。
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

// harmfulPreventionNear 在 focus 前查找劝阻动作，在 focus 后只查找救助结果。
// 输入 policy 分别提供前置防范词和后置处置词；返回首个命中标记，未命中时返回空串。
// 两个方向不能混用，否则“你去跳楼，别再出现”中的“别”会被错误理解为阻止跳楼。
func harmfulPreventionNear(value, focus string,
	policy appconfig.CommentModerationHarmfulValuePolicyConfig,
) string {
	_, suffix := relationWindows(value, focus, 12, 10)
	for _, marker := range policy.PreventionMarkers {
		marker = compactText(normalizeText(marker))
		if marker != "" && relationMarkerBefore(value, focus, []string{marker}, 6) {
			return marker
		}
	}
	for _, marker := range policy.PostventionMarkers {
		marker = compactText(normalizeText(marker))
		if marker != "" && strings.Contains(suffix, marker) {
			return marker
		}
	}
	return ""
}

// harmfulIncitementNear 在 focus 附近查找策略定义的教唆前缀或后缀。
// 返回命中的指令词；未形成指令结构时返回空串。
func harmfulIncitementNear(value, focus string,
	policy appconfig.CommentModerationHarmfulValuePolicyConfig,
) string {
	prefix, suffix := relationWindows(value, focus, 12, 18)
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

// relationWindows 返回 focus 在 value 中的前后局部文本窗口。
// 输入 before 和 after 是字符数上限；focus 不存在时两个返回值均为完整规范化文本。
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

// isSelfHarmExpression 判断 match 是否为带第一人称意念标记的自伤表达。
// 输入 value 是分句、policy 提供代词和意念词；返回 true 表示表达自身风险而非教唆他人。
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

// nearestRelationPerson 查找 value 中最后出现的人称代词并判断其属于自己还是他人。
// 返回 "self"、"other" 或未命中时的空串。
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

// harmfulRelationTarget 从 focus 前后窗口中寻找受有害动作影响的最近对象。
// 输入 policy 和 cfg 提供附加及通用人物目标；返回与动作字符距离最短的对象，
// 同距离时优先较长词项，避免“他们…你…去死”被配置顺序误导。
func harmfulRelationTarget(value, focus string, policy appconfig.CommentModerationHarmfulValuePolicyConfig,
	cfg appconfig.CommentModerationConfig,
) string {
	prefix, suffix := relationWindows(value, focus, 18, 18)
	targets := append([]string(nil), cfg.SemanticRules.RelationVocabulary.PersonTargets...)
	targets = append(targets, policy.AdditionalTargets...)
	return nearestHarmfulRelationTarget(prefix, suffix, targets)
}

// nearestHarmfulRelationTarget 比较 focus 前后的人物词项距离。
// prefix 和 suffix 分别是动作前后文本，targets 是可选对象；返回最近对象，未命中返回空串。
func nearestHarmfulRelationTarget(prefix, suffix string, targets []string) string {
	bestTarget := ""
	bestDistance := int(^uint(0) >> 1)
	for _, target := range targets {
		target = compactText(normalizeText(target))
		if target == "" {
			continue
		}
		if index := strings.LastIndex(prefix, target); index >= 0 {
			distance := len([]rune(prefix[index+len(target):]))
			if distance < bestDistance ||
				(distance == bestDistance && len([]rune(target)) > len([]rune(bestTarget))) {
				bestTarget = target
				bestDistance = distance
			}
		}
		if index := strings.Index(suffix, target); index >= 0 {
			distance := len([]rune(suffix[:index]))
			if distance < bestDistance ||
				(distance == bestDistance && len([]rune(target)) > len([]rune(bestTarget))) {
				bestTarget = target
				bestDistance = distance
			}
		}
	}
	return bestTarget
}

// harmfulTargetIsAddressed 判断 target 是否被评论直接称呼或置于明确指令结构中。
// 输入 value、focus、directive 描述完整语境；返回 true 表示有害动作确实指向该对象。
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

// harmfulConceptIsReferenced 判断 focus 后是否紧跟引用后缀。
// 输入 value 是分句、policy 提供引用标记；返回 true 表示只是在提及概念而非实施或教唆。
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

// criticalOutcomeNegated 判断 outcome 前的局部窗口是否包含任一否定 markers；返回对应结果。
func criticalOutcomeNegated(value, outcome string, markers []string) bool {
	prefix, _ := relationWindows(value, outcome, 6, 0)
	return containsAnyNormalized(prefix, markers)
}

// educationHasPromotionalConflict 判断风险科普语境是否同时出现行动推广或策略冲突词。
// 返回 true 表示不能按良性科普关系处理。
func educationHasPromotionalConflict(value string, cfg appconfig.CommentModerationConfig,
	policy appconfig.CommentModerationHarmfulValuePolicyConfig,
) bool {
	if containsAnyNormalized(value, cfg.SemanticRules.Contexts.ActionableMarkers) ||
		matchesAnySemanticPattern(value, cfg.SemanticRules.Contexts.ActionablePatterns) {
		return true
	}
	return containsAnyNormalized(value, policy.PromotionConflicts)
}

// evidenceObjectInClause 从 evidence 中提取实际出现在 clause 里的最长有效词项并返回。
func evidenceObjectInClause(clause, value string) string {
	best := ""
	for _, term := range evidenceTerms(value) {
		if strings.Contains(clause, term) && len([]rune(term)) > len([]rune(best)) {
			best = term
		}
	}
	return best
}

// matchResult 将 match 的辅助摄入动作与规范概念组合成关系结果并返回。
func matchResult(match harmfulRelationMatch) string {
	if match.Auxiliary != "" {
		return match.Auxiliary + match.Canonical
	}
	return match.Canonical
}

// matchEvidence 构造 match 的可审计证据；观测形式与规范形式不同时返回“观测→还原结果”。
func matchEvidence(match harmfulRelationMatch) string {
	result := matchResult(match)
	if match.Observed != match.Canonical {
		return match.Observed + "→" + result
	}
	return result
}

// minProbability 返回 left 和 right 中较小的概率值。
func minProbability(left, right float64) float64 {
	if left < right {
		return left
	}
	return right
}

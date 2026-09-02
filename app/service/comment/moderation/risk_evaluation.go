package moderation

import (
	"sort"
	"strings"

	appconfig "meta-api/config"
)

// 立场评价把风险行为视为被评论者评价的对象，而不是评论者正在实施的动作。
// 该语法不绑定具体风险分类：检测器提供领域对象和分类，本层只处理判断词、负面结果和治理动作等稳定语义原语。
const relationSubtypeStanceEvaluation = "stance_evaluation"

// stanceEvaluationMatch 保存立场评价中的判断谓词、结果、立场、对象边界和置信度。
type stanceEvaluationMatch struct {
	Predicate  string
	Outcome    string
	Stance     string
	Boundary   int
	Confidence float64
}

// appendRiskEvaluationRelations 将“风险行为 + 判断词 + 负面结果/治理动作”转换为反向评价关系。
// 输入 clauseID、clause、evidence、cfg 提供分句与策略；识别出的关系通过 appendRelation 回调输出。
func appendRiskEvaluationRelations(clauseID int, clause NormalizedComment, evidence []Evidence,
	cfg appconfig.CommentModerationConfig, appendRelation func(SemanticRelation),
) {
	match, ok := matchStanceEvaluation(clause, cfg)
	if !ok {
		return
	}
	bestObjectByCategory := make(map[string]string)
	rememberObject := func(category, object string) {
		category = strings.ToLower(strings.TrimSpace(category))
		object = strings.TrimSpace(object)
		if category == "" || object == "" {
			return
		}
		if current := bestObjectByCategory[category]; len([]rune(object)) > len([]rune(current)) {
			bestObjectByCategory[category] = object
		}
	}
	for _, item := range evidence {
		if item.Polarity != "positive" || item.Category == "" || item.Category == "benign_context" ||
			item.Clause > 0 && item.Clause != clauseID {
			continue
		}
		object := evaluatedBehaviorInClause(clause.Compact, item.Value, match.Boundary)
		if object == "" {
			continue
		}
		rememberObject(item.Category, object)
	}
	// 良性评价分句中的组合信号会被主动抑制，因此这里直接使用同一套主体词和谓词重建被评价行为，
	// 不依赖可能已经被移除的信号，同时保持语义语法与具体分类解耦。
	for _, rule := range cfg.CombinationRules {
		category := strings.TrimSpace(rule.Category)
		if category == "" {
			category = strings.TrimSpace(rule.ID)
		}
		for _, object := range containedRelationTerms(clause.Compact, rule.Subjects) {
			for _, action := range containedRelationTerms(clause.Compact, rule.Predicates) {
				if object == action || !relationTermsCanRelate(clause.Normalized, object, action, cfg) {
					continue
				}
				rememberObject(category,
					evaluatedBehaviorInClause(clause.Compact, object+"+"+action, match.Boundary))
			}
		}
	}
	for category, object := range bestObjectByCategory {
		appendRelation(SemanticRelation{
			Clause: clauseID, Type: RelationTypeEvaluation, Subject: "评论者", Action: "评价",
			Object: object, Predicate: match.Predicate, Result: match.Outcome, Stance: match.Stance,
			Category: category, Subtype: relationSubtypeStanceEvaluation,
			Evidence:   object + "+" + match.Predicate + "+" + match.Outcome,
			Confidence: match.Confidence,
		})
	}
}

// isRiskEvaluationSemanticClause 判断 clause 是否满足风险行为立场评价语法；返回对应结果。
func isRiskEvaluationSemanticClause(clause NormalizedComment, cfg appconfig.CommentModerationConfig) bool {
	_, ok := matchStanceEvaluation(clause, cfg)
	return ok
}

// matchStanceEvaluation 在 clause 中查找完整的判断结果或治理动作结构。
// 输入 cfg 提供评价词、结果词及冲突规则；返回最佳匹配和成功标记，疑问或推广语境返回 false。
func matchStanceEvaluation(clause NormalizedComment,
	cfg appconfig.CommentModerationConfig,
) (stanceEvaluationMatch, bool) {
	value := clause.Compact
	policy := cfg.SemanticRules.RiskEvaluation
	if value == "" || stanceEvaluationIsQuestion(value, policy) {
		return stanceEvaluationMatch{}, false
	}

	best := stanceEvaluationMatch{}
	for _, class := range policy.Outcomes {
		for _, root := range class.Roots {
			root = compactText(normalizeText(root))
			start := strings.Index(value, root)
			if root == "" || start < 0 {
				continue
			}
			outcome := expandStanceOutcome(value, start, root, policy.OutcomeSuffixes)
			if criticalOutcomeNegated(value, outcome, policy.OutcomeNegations) ||
				stanceEvaluationHasPromotionalConflict(value, outcome, cfg) {
				continue
			}
			predicate := precedingStancePredicate(value, start, policy)
			if predicate == "" {
				continue
			}
			if containsAnyNormalized(predicate, policy.DemonstrativePredicates) {
				_, suffix := relationWindows(value, outcome, 0, 20)
				if !containsAnyNormalized(suffix, policy.PostOutcomeRejections) {
					continue
				}
			}
			candidate := stanceEvaluationMatch{
				Predicate: predicate, Outcome: outcome, Stance: class.Stance,
				Boundary: start - len(predicate), Confidence: 0.97,
			}
			if candidate.Boundary < 0 {
				candidate.Boundary = start
			}
			if len([]rune(candidate.Outcome)) > len([]rune(best.Outcome)) {
				best = candidate
			}
		}
	}
	if best.Outcome != "" {
		return best, true
	}

	for _, action := range policy.GovernanceActions {
		action = compactText(normalizeText(action))
		index := strings.LastIndex(value, action)
		if action == "" || index <= 0 || stanceEvaluationHasPromotionalConflict(value, action, cfg) {
			continue
		}
		prefix := value[:index]
		modal := longestSuffix(prefix, policy.GovernanceModals)
		if modal == "" && action != "禁止" && action != "反对" && action != "抵制" {
			continue
		}
		predicate := modal
		if predicate == "" {
			predicate = action
		}
		return stanceEvaluationMatch{
			Predicate: predicate, Outcome: action, Stance: RelationStanceCondemnation,
			Boundary: index - len(modal), Confidence: 0.96,
		}, true
	}
	return stanceEvaluationMatch{}, false
}

// expandStanceOutcome 从 root 起点向后拼接 suffixes 中最长的结果后缀。
// 输入 value 是分句、start 是字节下标；返回完整结果词，越界时返回 root。
func expandStanceOutcome(value string, start int, root string, suffixes []string) string {
	end := start + len(root)
	if end > len(value) {
		return root
	}
	return root + longestPrefix(value[end:], suffixes)
}

// precedingStancePredicate 在结果位置 outcomeStart 之前查找最长判断词或警示词。
// 输入 value 和 policy 分别为分句及策略；返回谓词，位置无效或未命中时返回空串。
func precedingStancePredicate(value string, outcomeStart int,
	policy appconfig.CommentModerationRiskEvaluationConfig,
) string {
	if outcomeStart < 0 || outcomeStart > len(value) {
		return ""
	}
	prefix := value[:outcomeStart]
	if predicate := longestSuffix(prefix, policy.JudgmentPredicates); predicate != "" {
		return predicate
	}
	return longestSuffix(prefix, policy.WarningPredicates)
}

// longestPrefix 在 value 开头匹配 candidates 中规范化后的最长词项并返回。
func longestPrefix(value string, candidates []string) string {
	best := ""
	for _, candidate := range candidates {
		candidate = compactText(normalizeText(candidate))
		if strings.HasPrefix(value, candidate) && len([]rune(candidate)) > len([]rune(best)) {
			best = candidate
		}
	}
	return best
}

// longestSuffix 在 value 末尾匹配 candidates 中规范化后的最长词项并返回。
func longestSuffix(value string, candidates []string) string {
	best := ""
	for _, candidate := range candidates {
		candidate = compactText(normalizeText(candidate))
		if strings.HasSuffix(value, candidate) && len([]rune(candidate)) > len([]rune(best)) {
			best = candidate
		}
	}
	return best
}

// evaluatedBehaviorInClause 在评价边界 boundary 之前定位 evidence 对应的被评价行为。
// 输入 clause 是紧凑分句；返回覆盖证据词的最小文本片段，片段过长时退化为最后一个词项。
func evaluatedBehaviorInClause(clause, evidence string, boundary int) string {
	type locatedTerm struct {
		start int
		end   int
	}
	located := make([]locatedTerm, 0, 2)
	for _, term := range evidenceTerms(evidence) {
		index := strings.Index(clause, term)
		if index < 0 || index >= boundary {
			continue
		}
		end := index + len(term)
		if end <= boundary {
			located = append(located, locatedTerm{start: index, end: end})
		}
	}
	if len(located) == 0 {
		return ""
	}
	sort.Slice(located, func(i, j int) bool { return located[i].start < located[j].start })
	start, end := located[0].start, located[0].end
	for _, term := range located[1:] {
		if term.start < start {
			start = term.start
		}
		if term.end > end {
			end = term.end
		}
	}
	if end > boundary || len([]rune(clause[start:end])) > 24 {
		last := located[len(located)-1]
		return clause[last.start:last.end]
	}
	return clause[start:end]
}

// stanceEvaluationIsQuestion 判断 value 是否包含配置疑问标记或以“吗”结尾；返回对应结果。
func stanceEvaluationIsQuestion(value string, policy appconfig.CommentModerationRiskEvaluationConfig) bool {
	return containsAnyNormalized(value, policy.QuestionMarkers) || strings.HasSuffix(value, "吗")
}

// stanceEvaluationHasPromotionalConflict 判断 focus 后是否出现推广动作、行动模式或转折推广结构。
// 输入 value 是完整分句、cfg 是语义配置；返回 true 表示评价关系不能作为良性反证。
func stanceEvaluationHasPromotionalConflict(value, focus string,
	cfg appconfig.CommentModerationConfig,
) bool {
	policy := cfg.SemanticRules.RiskEvaluation
	if containsAnyNormalized(value, policy.PromotionMarkers) {
		return true
	}
	_, suffix := relationWindows(value, focus, 0, 48)
	if containsAnyNormalized(suffix, cfg.SemanticRules.Contexts.ActionableMarkers) ||
		matchesAnySemanticPattern(suffix, cfg.SemanticRules.Contexts.ActionablePatterns) {
		return true
	}
	return containsAnyNormalized(suffix, policy.PromotionContrastMarkers) &&
		containsAnyNormalized(suffix, policy.PromotionActionMarkers)
}

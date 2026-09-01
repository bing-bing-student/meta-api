package moderation

import (
	"sort"
	"strings"

	appconfig "meta-api/config"
)

// A stance evaluation treats the risky behaviour as the object being judged,
// rather than as an action performed by the commenter. The grammar is kept
// category-independent: detectors provide the domain object/category, while
// this layer only understands stable semantic primitives such as copulas,
// negative outcomes and governance actions.
const relationSubtypeStanceEvaluation = "stance_evaluation"

type stanceOutcomeClass = appconfig.CommentModerationStanceOutcomeConfig

type stanceEvaluationMatch struct {
	Predicate  string
	Outcome    string
	Stance     string
	Boundary   int
	Confidence float64
}

var (
	stanceEvaluationOutcomes = []stanceOutcomeClass{
		{Roots: []string{"诈骗", "骗局", "骗术", "陷阱", "套路"}, Stance: RelationStanceWarning},
		{Roots: []string{"非法", "违法", "违规", "犯罪", "黑产", "灰产"}, Stance: RelationStanceCondemnation},
		{Roots: []string{"有害", "危害", "危险", "不道德", "可耻", "缺德"}, Stance: RelationStanceCondemnation},
	}
	stanceOutcomeSuffixes    = []string{"产业链", "产业", "行业", "行为", "活动", "生意", "内容", "问题"}
	stanceJudgmentPredicates = []string{
		"基本都是", "很可能是", "大概率是", "本质上是", "本质是", "大多是", "往往是",
		"可能是", "多半是", "基本是", "都属于", "属于", "算是", "构成", "就是", "都是", "是", "这种", "这类",
	}
	stanceDemonstrativePredicates = []string{"这种", "这类"}
	stancePostOutcomeRejections   = []string{
		"不可信", "不靠谱", "不能信", "不要信", "不要相信", "别信", "应举报", "应该举报", "建议举报", "必须举报",
	}
	stanceWarningPredicates = []string{"谨防", "警惕", "小心", "当心", "防范", "远离"}
	stanceGovernanceActions = []string{
		"严厉打击", "依法打击", "坚决抵制", "主动举报", "禁止", "取缔", "打击", "抵制", "举报", "反对", "谴责", "批判",
	}
	stanceGovernanceModals = []string{"应当", "应该", "必须", "需要", "要", "建议", "值得"}
	stancePromotionMarkers = []string{
		"我这边", "我这里", "我有", "我能", "手上还有", "找我", "给我", "加我", "联系我",
		"我替你", "我帮你", "本人接", "我的渠道", "我的项目", "我的资源", "可以接", "能够接",
	}
)

// resolvedRiskEvaluationPolicy 优先使用外部策略包；空配置只出现在单元测试、
// 配置加载失败前的兜底路径中，此时沿用保守的内置安全基线，避免热更新把识别静默关闭。
func resolvedRiskEvaluationPolicy(cfg appconfig.CommentModerationConfig) appconfig.CommentModerationRiskEvaluationConfig {
	policy := cfg.SemanticRules.RiskEvaluation
	if len(policy.Outcomes) > 0 && len(policy.JudgmentPredicates) > 0 && len(policy.QuestionMarkers) > 0 {
		return policy
	}
	return appconfig.CommentModerationRiskEvaluationConfig{
		Outcomes:                 stanceEvaluationOutcomes,
		OutcomeSuffixes:          stanceOutcomeSuffixes,
		JudgmentPredicates:       stanceJudgmentPredicates,
		DemonstrativePredicates:  stanceDemonstrativePredicates,
		PostOutcomeRejections:    stancePostOutcomeRejections,
		WarningPredicates:        stanceWarningPredicates,
		GovernanceActions:        stanceGovernanceActions,
		GovernanceModals:         stanceGovernanceModals,
		PromotionMarkers:         stancePromotionMarkers,
		QuestionMarkers:          []string{"是不是", "是否", "算不算", "属不属于", "违法吗", "合法吗", "违规吗", "犯罪吗", "是不是骗局"},
		PromotionContrastMarkers: []string{"但是", "不过", "然而", "可是", "例外"},
		PromotionActionMarkers:   []string{"需要", "想要", "可以", "提供", "领取", "私聊", "联系", "加入", "接单"},
	}
}

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
	// Combination signals are intentionally suppressed in a benign evaluation
	// clause, so reconstruct the evaluated behaviour from the same configured
	// subject/predicate vocabulary instead of depending on a signal that may no
	// longer exist. The semantic grammar remains category-independent.
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

func isRiskEvaluationSemanticClause(clause NormalizedComment, cfg appconfig.CommentModerationConfig) bool {
	_, ok := matchStanceEvaluation(clause, cfg)
	return ok
}

func matchStanceEvaluation(clause NormalizedComment,
	cfg appconfig.CommentModerationConfig,
) (stanceEvaluationMatch, bool) {
	value := clause.Compact
	policy := resolvedRiskEvaluationPolicy(cfg)
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
			if criticalOutcomeNegated(value, outcome,
				resolvedHarmfulValuePolicy(cfg.SemanticRules.HarmfulValuePolicy).OutcomeNegations) ||
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

func expandStanceOutcome(value string, start int, root string, suffixes []string) string {
	end := start + len(root)
	if end > len(value) {
		return root
	}
	return root + longestPrefix(value[end:], suffixes)
}

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

func stanceEvaluationIsQuestion(value string, policy appconfig.CommentModerationRiskEvaluationConfig) bool {
	return containsAnyNormalized(value, policy.QuestionMarkers) || strings.HasSuffix(value, "吗")
}

func stanceEvaluationHasPromotionalConflict(value, focus string,
	cfg appconfig.CommentModerationConfig,
) bool {
	policy := resolvedRiskEvaluationPolicy(cfg)
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

package moderation

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"

	appconfig "meta-api/config"
)

// candidateRelationTerm 表示参与关系组合的规范词、实际观测形式、置信度及是否来自改写候选。
type candidateRelationTerm struct {
	Canonical  string
	Observed   string
	Confidence float64
	Candidate  bool
}

// analyzeSemanticRelations 将规则证据和改写候选组织为“主体—动作—对象—结果—立场”关系。
// 输入 text、candidates、evidence、cfg 分别为评论、还原候选、已有证据和策略；返回去重、编号且概率规范化的关系集合。
func analyzeSemanticRelations(text NormalizedComment, candidates []RewriteCandidate, evidence []Evidence,
	cfg appconfig.CommentModerationConfig,
) []SemanticRelation {
	clauses := semanticClauses(text, cfg)
	vocabulary := cfg.SemanticRules.RelationVocabulary
	result := make([]SemanticRelation, 0, len(clauses)*2)
	seen := make(map[string]struct{})
	appendRelation := func(relation SemanticRelation) {
		relation.Type = strings.ToLower(strings.TrimSpace(relation.Type))
		relation.Subject = strings.TrimSpace(relation.Subject)
		relation.Action = strings.TrimSpace(relation.Action)
		relation.Object = strings.TrimSpace(relation.Object)
		relation.Predicate = strings.TrimSpace(relation.Predicate)
		relation.Result = strings.TrimSpace(relation.Result)
		relation.Stance = strings.ToLower(strings.TrimSpace(relation.Stance))
		relation.Category = strings.ToLower(strings.TrimSpace(relation.Category))
		relation.Subtype = strings.ToLower(strings.TrimSpace(relation.Subtype))
		relation.Evidence = strings.TrimSpace(relation.Evidence)
		if relation.Clause <= 0 || relation.Clause > len(clauses) ||
			(relation.Action == "" && relation.Object == "" && relation.Result == "") {
			return
		}
		if relation.Type == "" && relation.Action != "" {
			relation.Type = RelationTypeAction
		}
		if relation.Type == RelationTypeAction && relation.Action != "" && relation.Subject == "" {
			relation.Inferred = true
			relation.Confidence = minProbability(relation.Confidence, 0.72)
		}
		if relation.Stance == "" {
			switch {
			case relation.Subtype == relationSubtypeSelfHarmExpression:
				relation.Type = RelationTypeExpression
				relation.Stance = RelationStanceSelfConcern
			case relation.Negated:
				relation.Stance = RelationStanceRejection
			case relation.Quoted || relation.Reported:
				relation.Stance = RelationStanceReporting
			case relation.Type == RelationTypeAction:
				relation.Stance = RelationStanceActionable
			}
		}
		key := fmt.Sprintf("%d\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s", relation.Clause,
			relation.Type, relation.Category, relation.Subtype, relation.Subject, relation.Action,
			relation.Object, relation.Predicate, relation.Result, relation.Stance)
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		relation.ID = fmt.Sprintf("r%03d", len(result)+1)
		relation.Confidence = clampProbability(relation.Confidence)
		result = append(result, relation)
	}

	for clauseIndex, clause := range clauses {
		clauseID := clauseIndex + 1
		benignClause := isBenignSemanticClause(clause, cfg) || isRiskEducationSemanticClause(clause, cfg)
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
					relation := buildClauseRelation(clauseID, clause, inferRelationActor(clause.Normalized, action, cfg),
						action, object, category, object+"+"+action, cfg, 0.9)
					if benignClause {
						relation.Reported = true
					}
					appendRelation(relation)
				}
			}
		}
		appendCandidateCombinationRelations(clauseID, clause, candidates, cfg, appendRelation)
		appendRiskEvaluationRelations(clauseID, clause, evidence, cfg, appendRelation)
		appendHarmfulValueRelations(clauseID, clause, candidates, evidence, cfg, appendRelation)
		for _, marker := range containedRelationTerms(clause.Compact, cfg.SemanticRules.AbusePolicy.SevereMarkers) {
			scope := relationTextScope(clause.Normalized, marker)
			target := firstContainedRelationTerm(scope, vocabulary.PersonTargets)
			if target == "" && firstContainedRelationTerm(scope, vocabulary.ContentTargets) != "" {
				appendRelation(SemanticRelation{Clause: clauseID, Object: marker, Category: "abuse",
					Evidence: marker, Confidence: 0.94})
				continue
			}
			if target == "" {
				target = marker
			}
			relation := buildClauseRelation(clauseID, clause, "评论者", "严重攻击", target,
				"abuse", marker, cfg, 0.94)
			relation.Negated = relationNegatedNear(clause.Normalized, marker, cfg)
			relation.Reported = relationReportedNear(clause.Normalized, marker, cfg)
			appendRelation(relation)
		}
	}
	analyzeDiscourseCarryRelations(clauses, cfg, appendRelation)

	for _, candidate := range candidates {
		if candidate.Role == CandidateRoleSubject || candidate.Role == CandidateRolePredicate {
			continue
		}
		clauseID := candidate.Clause
		if clauseID <= 0 || clauseID > len(clauses) {
			clauseID = locateRelationClause(clauses, candidate.Observed, candidate.Text)
		}
		if clauseID <= 0 || clauseID > len(clauses) {
			continue
		}
		clause := clauses[clauseID-1]
		observedLiteral := strings.TrimSpace(candidate.Observed)
		observed := compactText(normalizeText(observedLiteral))
		if observed == "" {
			observed = compactText(normalizeText(candidate.Text))
		}
		action := nearestRelationAction(clause.Normalized, observed, candidate.Category, cfg)
		subject := inferRelationActor(clause.Normalized, action, cfg)
		object := candidate.Text
		resultText := ""
		if candidate.Category == "abuse" {
			scope := relationTextScope(clause.Normalized, observed)
			target := firstContainedRelationTerm(scope, vocabulary.PersonTargets)
			severe := containsAnyNormalized(compactText(normalizeText(candidate.Text)),
				cfg.SemanticRules.AbusePolicy.SevereMarkers)
			if target != "" {
				subject = "评论者"
				action = "辱骂"
				if severe {
					action = "严重攻击"
				}
				object = target
				resultText = candidate.Text
			} else if candidate.Method != "pinyin_homophone" && severe &&
				firstContainedRelationTerm(scope, vocabulary.ContentTargets) == "" {
				subject = "评论者"
				action = "严重攻击"
				object = candidate.Text
			}
		}
		relation := buildClauseRelation(clauseID, clause, subject, action, object, candidate.Category,
			candidate.Observed+"→"+candidate.Text, cfg, candidate.Confidence)
		if compactText(normalizeText(observedLiteral)) == "" {
			relation.Negated = false
		} else {
			relation.Negated = relationNegatedNear(clause.Normalized, observed, cfg)
		}
		relation.Reported = isRiskEducationSemanticClause(clause, cfg) ||
			isRiskEvaluationSemanticClause(clause, cfg) ||
			relationReportedNear(clause.Normalized, observed, cfg)
		if resultText != "" {
			relation.Result = resultText
		}
		appendRelation(relation)
	}

	for _, item := range evidence {
		if item.Polarity != "positive" || item.Source == SourceContext || item.Source == SourceLocalContext {
			continue
		}
		clauseID := item.Clause
		if clauseID <= 0 || clauseID > len(clauses) {
			clauseID = locateRelationClause(clauses, item.Value)
		}
		if clauseID <= 0 || clauseID > len(clauses) {
			continue
		}
		clause := clauses[clauseID-1]
		terms := evidenceTerms(item.Value)
		object := item.Value
		action := ""
		if len(terms) >= 2 {
			if !relationTermsCanRelate(clause.Normalized, terms[0], terms[1], cfg) {
				continue
			}
			object = terms[0]
			action = terms[1]
		} else if len(terms) == 1 {
			object = terms[0]
		}
		if action == "" {
			action = nearestRelationAction(clause.Normalized, compactText(normalizeText(object)), item.Category, cfg)
		}
		relation := buildClauseRelation(clauseID, clause, inferRelationActor(clause.Normalized, action, cfg),
			action, object, item.Category, item.Value, cfg, item.Confidence)
		if isRiskEducationSemanticClause(clause, cfg) || isRiskEvaluationSemanticClause(clause, cfg) {
			relation.Reported = true
		}
		appendRelation(relation)
	}

	return result
}

// appendCandidateCombinationRelations 将候选还原词与同分句的直接词项组合成语义关系。
// 输入 clauseID 标识分句，candidates 和 cfg 提供候选及组合规则；结果通过 appendRelation 回调输出。
func appendCandidateCombinationRelations(clauseID int, clause NormalizedComment,
	candidates []RewriteCandidate, cfg appconfig.CommentModerationConfig,
	appendRelation func(SemanticRelation),
) {
	for _, rule := range cfg.CombinationRules {
		category := strings.TrimSpace(rule.Category)
		if category == "" {
			category = strings.TrimSpace(rule.ID)
		}
		objects := directCandidateRelationTerms(clause.Compact, rule.Subjects)
		actions := directCandidateRelationTerms(clause.Compact, rule.Predicates)
		for _, candidate := range candidates {
			if candidate.Clause != clauseID || !strings.EqualFold(candidate.Category, category) {
				continue
			}
			canonical := compactText(normalizeText(candidate.Text))
			observed := compactText(normalizeText(candidate.Observed))
			if canonical == "" || observed == "" {
				continue
			}
			term := candidateRelationTerm{
				Canonical: canonical, Observed: observed,
				Confidence: candidate.Confidence, Candidate: true,
			}
			switch candidate.Role {
			case CandidateRoleSubject:
				if relationRuleContainsTerm(rule.Subjects, canonical) {
					objects = append(objects, term)
				}
			case CandidateRolePredicate:
				if relationRuleContainsTerm(rule.Predicates, canonical) {
					actions = append(actions, term)
				}
			}
		}
		for _, object := range objects {
			for _, action := range actions {
				if !object.Candidate && !action.Candidate || object.Observed == action.Observed ||
					!relationTermsCanRelate(clause.Normalized, object.Observed, action.Observed, cfg) {
					continue
				}
				confidence := 0.9
				if object.Candidate {
					confidence = object.Confidence
				}
				if action.Candidate && (!object.Candidate || action.Confidence < confidence) {
					confidence = action.Confidence
				}
				if object.Candidate && action.Candidate {
					confidence *= 0.96
				}
				relation := SemanticRelation{
					Clause:   clauseID,
					Subject:  inferRelationActor(clause.Normalized, action.Observed, cfg),
					Action:   action.Canonical,
					Object:   object.Canonical,
					Result:   extractRelationResult(clause.Compact, cfg),
					Category: category,
					Evidence: candidateRelationEvidence(object, action),
					Negated:  relationNegatedNear(clause.Normalized, action.Observed, cfg),
					Quoted:   relationTermsQuoted(clause.Raw, object.Observed, action.Observed),
					Reported: isBenignSemanticClause(clause, cfg) ||
						relationReportedNear(clause.Normalized, action.Observed, cfg),
					Confidence: confidence,
				}
				appendRelation(relation)
			}
		}
	}
}

// directCandidateRelationTerms 将 value 中直接命中的 terms 转换为非候选关系词项并返回。
func directCandidateRelationTerms(value string, terms []string) []candidateRelationTerm {
	contained := containedRelationTerms(value, terms)
	result := make([]candidateRelationTerm, 0, len(contained))
	for _, term := range contained {
		result = append(result, candidateRelationTerm{
			Canonical: term, Observed: term, Confidence: 0.9,
		})
	}
	return result
}

// relationRuleContainsTerm 判断 canonical 是否与 terms 中任一规范化词项完全相等；返回对应结果。
func relationRuleContainsTerm(terms []string, canonical string) bool {
	for _, term := range terms {
		if compactText(normalizeText(term)) == canonical {
			return true
		}
	}
	return false
}

// candidateRelationEvidence 格式化 object 与 action 组成的关系证据。
// 对发生还原的候选保留“观测→规范”形式；返回两部分以加号连接的字符串。
func candidateRelationEvidence(object, action candidateRelationTerm) string {
	format := func(term candidateRelationTerm) string {
		if term.Candidate && term.Observed != term.Canonical {
			return term.Observed + "→" + term.Canonical
		}
		return term.Canonical
	}
	return format(object) + "+" + format(action)
}

// buildClauseRelation 根据分句、主体、动作、对象及分类创建基础语义关系。
// 输入 cfg 用于结果、否定、引用和转述判断，confidence 是初始置信度；返回尚未编号的关系。
func buildClauseRelation(clauseID int, clause NormalizedComment, subject, action, object, category,
	evidence string, cfg appconfig.CommentModerationConfig, confidence float64,
) SemanticRelation {
	focus := action
	if focus == "" {
		focus = object
	}
	return SemanticRelation{
		Clause:     clauseID,
		Subject:    subject,
		Action:     action,
		Object:     object,
		Result:     extractRelationResult(clause.Compact, cfg),
		Category:   category,
		Evidence:   evidence,
		Negated:    relationNegatedNear(clause.Normalized, focus, cfg),
		Quoted:     relationTermsQuoted(clause.Raw, evidence, action, object),
		Reported:   relationReportedNear(clause.Normalized, focus, cfg),
		Confidence: confidence,
	}
}

// containedRelationTerms 返回 terms 中所有实际包含于 value 的规范化非空词项。
func containedRelationTerms(value string, terms []string) []string {
	result := make([]string, 0, 2)
	for _, term := range terms {
		term = compactText(normalizeText(term))
		if term != "" && strings.Contains(value, term) {
			result = append(result, term)
		}
	}
	return result
}

// firstContainedRelationTerm 返回 terms 中首个包含于 value 的规范化词项；未命中时返回空串。
func firstContainedRelationTerm(value string, terms []string) string {
	for _, term := range terms {
		term = compactText(normalizeText(term))
		if term != "" && strings.Contains(value, term) {
			return term
		}
	}
	return ""
}

// nearestRelationAction 在 value 中寻找与 object 距离最近、范围可关联且属于 category 的动作词。
// 输入 cfg 提供行动标记和组合谓词；返回最佳动作，未找到时返回空串。
func nearestRelationAction(value, object, category string, cfg appconfig.CommentModerationConfig) string {
	value = normalizeText(value)
	markers := append([]string(nil), cfg.SemanticRules.Contexts.ActionableMarkers...)
	markers = append(markers, semanticPatternMatches(value, cfg.SemanticRules.Contexts.ActionablePatterns)...)
	for _, rule := range cfg.CombinationRules {
		ruleCategory := strings.TrimSpace(rule.Category)
		if ruleCategory == "" {
			ruleCategory = strings.TrimSpace(rule.ID)
		}
		if !strings.EqualFold(ruleCategory, category) {
			continue
		}
		markers = append(markers, rule.Predicates...)
	}
	objectIndex := strings.Index(value, object)
	best := ""
	bestDistance := len([]rune(value)) + 1
	for _, marker := range markers {
		marker = compactText(normalizeText(marker))
		if len([]rune(marker)) == 1 && containsHan(marker) {
			continue
		}
		index := relationTermIndex(value, marker)
		if marker == "" || index < 0 {
			continue
		}
		if objectIndex >= 0 && !relationTermsCanRelate(value, object, marker, cfg) {
			continue
		}
		if objectIndex >= 0 && byteRangesOverlap(index, index+len(marker), objectIndex, objectIndex+len(object)) {
			continue
		}
		distance := 0
		if objectIndex >= 0 {
			distance = absInt(len([]rune(value[:index])) - len([]rune(value[:objectIndex])))
		}
		if best == "" || distance < bestDistance || distance == bestDistance && len([]rune(marker)) > len([]rune(best)) {
			best = marker
			bestDistance = distance
		}
	}
	return best
}

// byteRangesOverlap 判断两个左闭右开字节区间是否相交；返回对应结果。
func byteRangesOverlap(leftStart, leftEnd, rightStart, rightEnd int) bool {
	return leftStart < rightEnd && rightStart < leftEnd
}

// relationTermIndex 在 value 中查找 marker 的首个合法位置。
// 对纯 ASCII marker 额外验证单词边界；返回字节下标，未找到时返回 -1。
func relationTermIndex(value, marker string) int {
	start := 0
	for start <= len(value) {
		index := strings.Index(value[start:], marker)
		if index < 0 {
			return -1
		}
		index += start
		if !isASCIIWord(marker) || asciiWordBoundary(value, index, len(marker)) {
			return index
		}
		start = index + len(marker)
	}
	return -1
}

// isASCIIWord 判断 value 是否为非空且全部由 ASCII 字母数字组成；返回对应结果。
func isASCIIWord(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r > 127 || !isASCIIAlphaNumeric(r) {
			return false
		}
	}
	return true
}

// asciiWordBoundary 判断 value 中 index、size 指定的片段两侧是否均为 ASCII 单词边界；返回对应结果。
func asciiWordBoundary(value string, index, size int) bool {
	before := []rune(value[:index])
	after := []rune(value[index+size:])
	return (len(before) == 0 || !isASCIIAlphaNumeric(before[len(before)-1])) &&
		(len(after) == 0 || !isASCIIAlphaNumeric(after[0]))
}

// isASCIIAlphaNumeric 判断字符 r 是否为 ASCII 字母或数字；返回对应结果。
func isASCIIAlphaNumeric(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
}

// inferRelationActor 在 action 所在局部范围内查找其前方最近的配置主体词。
// 输入 value 是分句、cfg 提供主体词；返回主体，无法可靠推断时返回空串。
func inferRelationActor(value, action string, cfg appconfig.CommentModerationConfig) string {
	value = relationTextScope(value, action)
	focus := strings.Index(value, action)
	if focus < 0 {
		focus = len(value)
	}
	best := ""
	bestIndex := -1
	for _, actor := range cfg.SemanticRules.RelationVocabulary.Actors {
		index := strings.LastIndex(value[:focus], actor)
		if index > bestIndex {
			best = actor
			bestIndex = index
		}
	}
	if best != "" {
		return best
	}
	return ""
}

// locateRelationClause 在 clauses 中寻找包含任一 terms 的首个分句。
// 返回从 1 开始的分句编号；未找到时返回 0。
func locateRelationClause(clauses []NormalizedComment, terms ...string) int {
	for index, clause := range clauses {
		for _, term := range terms {
			term = compactText(normalizeText(term))
			if term != "" && strings.Contains(clause.Compact, term) {
				return index + 1
			}
		}
	}
	return 0
}

// relationMarkerBefore 判断 focus 前 maxDistance 个字符内是否存在任一 markers。
// 输入 value 是完整分句；返回 true 表示前置语义标记在允许窗口内。
func relationMarkerBefore(value, focus string, markers []string, maxDistance int) bool {
	value = relationTextScope(value, focus)
	focus = compactText(normalizeText(focus))
	focusIndex := strings.Index(value, focus)
	if focus == "" || focusIndex < 0 {
		focusIndex = len(value)
	}
	focusRunes := len([]rune(value[:focusIndex]))
	for _, marker := range markers {
		marker = compactText(normalizeText(marker))
		if marker == "" {
			continue
		}
		search := value[:focusIndex]
		index := strings.LastIndex(search, marker)
		if index < 0 {
			continue
		}
		distance := focusRunes - len([]rune(value[:index])) - len([]rune(marker))
		if distance >= 0 && distance <= maxDistance {
			return true
		}
	}
	return false
}

// relationMarkerAfter 判断 focus 后 maxDistance 个字符内是否存在任一 markers。
// 输入 value 是完整分句；返回 true 表示后置语义标记在允许窗口内。
func relationMarkerAfter(value, focus string, markers []string, maxDistance int) bool {
	scope := relationTextScope(value, focus)
	focus = compactText(normalizeText(focus))
	focusIndex := strings.Index(scope, focus)
	if focus == "" || focusIndex < 0 {
		return false
	}
	start := focusIndex + len(focus)
	for _, marker := range markers {
		marker = compactText(normalizeText(marker))
		if marker == "" {
			continue
		}
		index := strings.Index(scope[start:], marker)
		if index < 0 {
			continue
		}
		distance := len([]rune(scope[start : start+index]))
		if distance <= maxDistance {
			return true
		}
	}
	return false
}

// relationNegatedNear 判断 focus 是否受附近否定词或拒绝语境支配。
// 输入 value 是分句、cfg 提供否定及疑问词；返回 true 表示该关系应标记为否定。
func relationNegatedNear(value, focus string, cfg appconfig.CommentModerationConfig) bool {
	vocabulary := cfg.SemanticRules.RelationVocabulary
	markers := append([]string(nil), vocabulary.NegationMarkers...)
	markers = append(markers, cfg.SemanticRules.Contexts.RejectionMarkers...)
	interrogative := relationHasInterrogativePrefix(value, focus, cfg)
	if interrogative {
		markers = relationMarkersExcept(markers, "不是")
	}
	if relationMarkerBefore(value, focus, markers, 10) {
		return true
	}
	scope := relationTextScope(value, focus)
	if strings.HasPrefix(scope, "不过") || strings.HasPrefix(scope, "不但") || strings.HasPrefix(scope, "不仅") {
		return false
	}
	if interrogative {
		return false
	}
	return relationMarkerBefore(value, focus, vocabulary.ImmediateNegationMarkers, 1)
}

// relationHasInterrogativePrefix 判断 focus 前是否紧邻配置的疑问前缀；返回对应结果。
func relationHasInterrogativePrefix(value, focus string, cfg appconfig.CommentModerationConfig) bool {
	scope := relationTextScope(value, focus)
	focus = compactText(normalizeText(focus))
	index := strings.Index(scope, focus)
	if focus == "" || index < 0 {
		return false
	}
	prefix := scope[:index]
	for _, suffix := range cfg.SemanticRules.RelationVocabulary.InterrogativePrefixes {
		if strings.HasSuffix(prefix, suffix) {
			return true
		}
	}
	return false
}

// relationMarkersExcept 返回 markers 中除规范化后等于 excluded 以外的词项。
func relationMarkersExcept(markers []string, excluded string) []string {
	result := make([]string, 0, len(markers))
	excluded = compactText(normalizeText(excluded))
	for _, marker := range markers {
		if compactText(normalizeText(marker)) != excluded {
			result = append(result, marker)
		}
	}
	return result
}

// relationReportedNear 判断 focus 是否处于转述、治理或强报告语境。
// 输入 value 是分句、cfg 提供报告词和治理规则；返回 true 表示关系不是评论者直接实施。
func relationReportedNear(value, focus string, cfg appconfig.CommentModerationConfig) bool {
	scope := relationTextScope(value, focus)
	vocabulary := cfg.SemanticRules.RelationVocabulary
	if relationWeakReportingTargetsPerson(scope, vocabulary) {
		return false
	}
	if relationGovernanceContext(value, focus, cfg) {
		return true
	}
	markers := strongRelationReportingMarkers(cfg.SemanticRules.Contexts.ReportingMarkers,
		vocabulary.WeakReportingMarkers)
	return relationMarkerBefore(value, focus, markers, 16) || relationMarkerAfter(value, focus, markers, 18)
}

// relationWeakReportingTargetsPerson 判断弱报告词是否直接修饰人物目标。
// 输入 scope 是局部文本、vocabulary 提供词表；返回 true 时不能仅凭弱词认定为转述。
func relationWeakReportingTargetsPerson(scope string,
	vocabulary appconfig.CommentModerationRelationVocabularyConfig,
) bool {
	scope = compactText(normalizeText(scope))
	for _, marker := range vocabulary.WeakReportingMarkers {
		marker = compactText(normalizeText(marker))
		for _, target := range vocabulary.PersonTargets {
			target = compactText(normalizeText(target))
			if marker != "" && target != "" && strings.Contains(scope, marker+target) {
				return true
			}
		}
	}
	return false
}

// strongRelationReportingMarkers 从 markers 中剔除 weakMarkers，返回可独立支撑转述判断的强报告词。
func strongRelationReportingMarkers(markers, weakMarkers []string) []string {
	weak := make(map[string]struct{}, len(weakMarkers))
	for _, marker := range weakMarkers {
		weak[compactText(normalizeText(marker))] = struct{}{}
	}
	result := make([]string, 0, len(markers))
	for _, marker := range markers {
		if _, found := weak[compactText(normalizeText(marker))]; !found {
			result = append(result, marker)
		}
	}
	return result
}

// relationGovernanceContext 判断 focus 是否位于治理、禁止或反对风险行为的语境。
// 输入 value 是完整分句、cfg 提供治理模式；返回 true 表示可作为转述或反向关系处理。
func relationGovernanceContext(value, focus string, cfg appconfig.CommentModerationConfig) bool {
	full := compactText(normalizeText(value))
	scope := relationTextScope(value, focus)
	vocabulary := cfg.SemanticRules.RelationVocabulary
	if containsAnyNormalized(scope, vocabulary.ContrastMarkers) {
		return false
	}
	if containsAnyNormalized(scope, vocabulary.FirstPersonMarkers) {
		return false
	}
	if matchesAnySemanticPattern(full, vocabulary.GovernancePatterns) {
		return true
	}
	if !containsAnyNormalized(full, vocabulary.GovernanceMarkers) {
		return false
	}
	return containsAnyNormalized(full, cfg.SemanticRules.Contexts.RejectionMarkers)
}

// relationTextScope 返回 value 中包含 focus 的逗号局部片段。
// 找不到对应片段时返回完整规范化紧凑文本，用于限制关系词的作用范围。
func relationTextScope(value, focus string) string {
	focus = compactText(normalizeText(focus))
	for _, segment := range strings.FieldsFunc(value, func(r rune) bool { return r == '，' || r == ',' }) {
		compact := compactText(normalizeText(segment))
		if focus != "" && strings.Contains(compact, focus) {
			return compact
		}
	}
	return compactText(normalizeText(value))
}

// relationTermsShareScope 判断全部 terms 是否共同出现在 value 的同一逗号片段；返回对应结果。
func relationTermsShareScope(value string, terms ...string) bool {
	for _, segment := range strings.FieldsFunc(value, func(r rune) bool { return r == '，' || r == ',' }) {
		compact := compactText(normalizeText(segment))
		matched := true
		for _, term := range terms {
			term = compactText(normalizeText(term))
			if term == "" || !strings.Contains(compact, term) {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

// relationTermsCanRelate 判断 object 与 action 是否位于同一作用域，或可从相邻分句安全承接。
// 输入 value 是文本、cfg 提供良性及主体标记；返回 true 表示两词可组成同一关系。
func relationTermsCanRelate(value, object, action string, cfg appconfig.CommentModerationConfig) bool {
	if relationTermsShareScope(value, object, action) {
		return true
	}
	segments := strings.FieldsFunc(value, func(r rune) bool { return r == '，' || r == ',' })
	objectIndex, actionIndex := -1, -1
	for index, segment := range segments {
		compact := compactText(normalizeText(segment))
		if objectIndex < 0 && strings.Contains(compact, compactText(normalizeText(object))) {
			objectIndex = index
		}
		if strings.Contains(compact, compactText(normalizeText(action))) {
			actionIndex = index
		}
	}
	if objectIndex < 0 || actionIndex != objectIndex+1 {
		return false
	}
	actionScope := compactText(normalizeText(segments[actionIndex]))
	vocabulary := cfg.SemanticRules.RelationVocabulary
	if containsAnyNormalized(actionScope, vocabulary.FirstPersonMarkers) {
		return true
	}
	objectScope := compactText(normalizeText(segments[objectIndex]))
	benignMarkers := append([]string(nil), cfg.SemanticRules.Contexts.ReportingMarkers...)
	benignMarkers = append(benignMarkers, cfg.SemanticRules.Contexts.RejectionMarkers...)
	benignMarkers = append(benignMarkers, cfg.SemanticRules.Contexts.TechnicalMarkers...)
	benignMarkers = append(benignMarkers, cfg.SemanticRules.Contexts.UnambiguousBenignMarkers...)
	objectBenignMarkers := append([]string(nil), benignMarkers...)
	objectBenignMarkers = append(objectBenignMarkers, vocabulary.NegationMarkers...)
	return !containsAnyNormalized(objectScope, objectBenignMarkers) &&
		!containsAnyNormalized(actionScope, benignMarkers)
}

// analyzeDiscourseCarryRelations 分析相邻分句中“前句对象、后句第一人称动作”的承接关系。
// 输入 clauses 和 cfg 提供分句及组合规则；推断关系通过 appendRelation 回调输出。
func analyzeDiscourseCarryRelations(clauses []NormalizedComment, cfg appconfig.CommentModerationConfig,
	appendRelation func(SemanticRelation),
) {
	for index := 1; index < len(clauses); index++ {
		previous, current := clauses[index-1], clauses[index]
		vocabulary := cfg.SemanticRules.RelationVocabulary
		if !containsAnyNormalized(current.Compact, vocabulary.FirstPersonMarkers) {
			continue
		}
		for _, rule := range cfg.CombinationRules {
			category := strings.TrimSpace(rule.Category)
			if category == "" {
				category = strings.TrimSpace(rule.ID)
			}
			actions := append([]string(nil), rule.Predicates...)
			actions = append(actions, vocabulary.PromotionActions...)
			for _, object := range containedRelationTerms(previous.Compact, rule.Subjects) {
				for _, action := range containedRelationTerms(current.Compact, actions) {
					relation := buildClauseRelation(index+1, current, inferRelationActor(current.Normalized, action, cfg),
						action, object, category, object+"+"+action, cfg, 0.86)
					relation.Inferred = true
					appendRelation(relation)
				}
			}
		}
	}
}

// relationTermsQuoted 判断 terms 中任一词是否出现在 raw 的引号片段内；返回对应结果。
func relationTermsQuoted(raw string, terms ...string) bool {
	segments := quotedRelationSegments(raw)
	for _, segment := range segments {
		compact := compactText(normalizeText(segment))
		for _, term := range terms {
			term = compactText(normalizeText(term))
			if term != "" && strings.Contains(compact, term) {
				return true
			}
		}
	}
	return false
}

// quotedRelationSegments 提取 value 中成对中英文引号、书名号或反引号包围的文本并返回。
func quotedRelationSegments(value string) []string {
	closing := map[rune]rune{
		'“': '”', '‘': '’', '「': '」', '『': '』', '《': '》', '`': '`', '"': '"', '\'': '\'',
	}
	runes := []rune(value)
	segments := make([]string, 0, 2)
	for index := 0; index < len(runes); index++ {
		closeRune, ok := closing[runes[index]]
		if !ok {
			continue
		}
		start := index + 1
		for index = start; index < len(runes); index++ {
			if runes[index] == closeRune {
				if start < index {
					segments = append(segments, string(runes[start:index]))
				}
				break
			}
		}
	}
	return segments
}

// extractRelationResult 在 value 中查找配置的结果连接词，并返回连接词及其后最多 24 个字符。
// 未命中连接词时返回空串。
func extractRelationResult(value string, cfg appconfig.CommentModerationConfig) string {
	for _, connector := range cfg.SemanticRules.RelationVocabulary.ResultConnectors {
		index := strings.Index(value, connector)
		if index < 0 {
			continue
		}
		runes := []rune(value[index+len(connector):])
		if len(runes) > 24 {
			runes = runes[:24]
		}
		return connector + string(runes)
	}
	return ""
}

// relationIsCounterEvidence 判断 relation 是否属于否定、引用、转述、谴责、警告或拒绝关系；返回对应结果。
func relationIsCounterEvidence(relation SemanticRelation) bool {
	return relation.Negated || relation.Quoted || relation.Reported ||
		relation.Stance == RelationStanceCondemnation || relation.Stance == RelationStanceWarning ||
		relation.Stance == RelationStanceRejection || relation.Stance == RelationStanceReporting
}

// relationEvidence 将 relations 转换为可融合的正向风险证据或反向语境证据。
// 返回值会避免在同分类存在可执行关系时生成会与之冲突的全局反证。
func relationEvidence(relations []SemanticRelation) []Evidence {
	result := make([]Evidence, 0, len(relations))
	activeCategories := make(map[string]struct{})
	for _, relation := range relations {
		if relationIsActionableRisk(relation) {
			activeCategories[relation.Category] = struct{}{}
		}
	}
	for _, relation := range relations {
		if relationIsActionableRisk(relation) && (relation.Inferred || relation.Subtype != "" ||
			relation.Category == "abuse" && relation.Action == "严重攻击") {
			ruleID := "semantic_relation"
			group := compactText(normalizeText(relation.Evidence))
			if relation.Category == "abuse" && relation.Action == "严重攻击" {
				ruleID = "severe_attack_relation"
				group = "severe-attack"
			}
			if relation.Subtype != "" {
				ruleID = relation.Subtype
				group = relation.Subtype + ":" + group
			}
			result = append(result, Evidence{
				ID:               "relation-" + relation.ID,
				Source:           SourceSemantic,
				Category:         relation.Category,
				Polarity:         "positive",
				Confidence:       relation.Confidence,
				CorrelationGroup: fmt.Sprintf("clause:%d:%s:%s", relation.Clause, relation.Category, group),
				Value:            relation.Evidence,
				RuleID:           ruleID,
				Clause:           relation.Clause,
			})
		}
		if !relationIsCounterEvidence(relation) || relation.Category == "" {
			continue
		}
		if _, hasActiveRelation := activeCategories[relation.Category]; hasActiveRelation {
			continue
		}
		result = append(result, Evidence{
			ID:               "relation-" + relation.ID,
			Source:           SourceSemantic,
			Category:         relation.Category,
			Polarity:         "negative",
			Confidence:       relation.Confidence,
			CorrelationGroup: fmt.Sprintf("clause:%d:%s:relation-scope", relation.Clause, relation.Category),
			Value:            relation.Evidence,
			RuleID:           relationCounterRuleID(relation),
			Clause:           relation.Clause,
		})
	}
	return result
}

// relationIsActionableRisk 判断 relation 是否为分类明确且具备动作和对象的可执行风险关系；返回对应结果。
func relationIsActionableRisk(relation SemanticRelation) bool {
	return relation.Type != RelationTypeEvaluation && !relationIsCounterEvidence(relation) && relation.Category != "" &&
		relation.Action != "" && relation.Object != ""
}

// relationCounterRuleID 返回 relation 反向证据的规则标识，优先使用子类型，缺失时返回通用作用域标识。
func relationCounterRuleID(relation SemanticRelation) string {
	if relation.Subtype != "" {
		return relation.Subtype
	}
	return "relation_scope"
}

// relationFingerprint 对 relations 的结构字段排序并计算稳定 SHA-256 指纹。
// 返回值不包含评论原文；空关系集合返回空串。
func relationFingerprint(relations []SemanticRelation) string {
	if len(relations) == 0 {
		return ""
	}
	parts := make([]string, 0, len(relations))
	for _, relation := range relations {
		parts = append(parts, strings.Join([]string{
			relation.Type,
			relation.Category,
			relation.Subtype,
			relation.Subject,
			relation.Action,
			relation.Object,
			relation.Predicate,
			relation.Result,
			relation.Stance,
			fmt.Sprintf("n=%t,q=%t,r=%t,i=%t", relation.Negated, relation.Quoted, relation.Reported,
				relation.Inferred),
		}, "|"))
	}
	sort.Strings(parts)
	digest := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return fmt.Sprintf("%x", digest[:])
}

// RelationFingerprint 暴露供人工反馈策略使用的稳定关系结构键。
// 输入 relations 是语义关系集合；返回结构指纹，特意不纳入评论原文字面内容。
func RelationFingerprint(relations []SemanticRelation) string {
	return relationFingerprint(relations)
}

// relationIntentProfile 根据 relations 的可执行关系与反向关系汇总局部意图。
// 输入 text 和 cfg 用于无关系时的疑问、技术语境兜底；返回意图、置信度、良性概率、风险增益和解释。
func relationIntentProfile(relations []SemanticRelation, text NormalizedComment,
	cfg appconfig.CommentModerationConfig,
) localIntentProfile {
	active := make([]SemanticRelation, 0, len(relations))
	counter := make([]SemanticRelation, 0, len(relations))
	for _, relation := range relations {
		if relationIsCounterEvidence(relation) {
			counter = append(counter, relation)
		} else if relationIsActionableRisk(relation) {
			active = append(active, relation)
		}
	}
	for _, relation := range active {
		switch relation.Subtype {
		case relationSubtypeSelfHarmEncouragement, relationSubtypeDangerousBehaviorEncouragement,
			relationSubtypeDeathWish:
			return localIntentProfile{"harmful_incitement", 0.97, 0.01, 0.08,
				"分句关系显示评论者对明确对象实施自伤或危险行为诱导"}
		case relationSubtypeSelfHarmExpression:
			return localIntentProfile{"self_harm_expression", 0.88, 0.02, 0.18,
				"分句关系显示评论者可能在表达自伤意图，应保留人工关注"}
		}
		if relation.Category == "abuse" && (relation.Action == "辱骂" || relation.Action == "严重攻击") &&
			relation.Object != "" {
			return localIntentProfile{"targeted_abuse", 0.94, 0.01, 0.2, "分句关系显示评论者对明确对象实施辱骂"}
		}
		if relation.Category == "abuse" {
			continue
		}
		if relation.Action != "" && relation.Object != "" {
			confidence := relation.Confidence
			if confidence <= 0 {
				confidence = 0.7
			}
			explanation := "同一分句内形成明确主体、对象与动作关系"
			if relation.Inferred || relation.Subject == "" {
				explanation = "同一分句内形成对象与动作关系，但行为主体来自推断"
			}
			return localIntentProfile{"actionable", confidence, 0.02, 0.18, explanation}
		}
	}
	if len(active) > 0 && relationsAreUntargetedAbuseOpinions(active, cfg) {
		return localIntentProfile{"content_criticism", 0.88, 0.9, 0, "辱骂词指向内容评价，未形成对人攻击关系"}
	}
	if len(active) == 0 && len(counter) > 0 {
		for _, relation := range counter {
			if relation.Subtype == relationSubtypeStanceEvaluation {
				return localIntentProfile{"risk_evaluation", relation.Confidence, 0.98, 0,
					"评论对风险行为作出明确的欺诈、违法、危险或负面定性"}
			}
			if relation.Subtype == relationSubtypeRiskEducation {
				return localIntentProfile{"risk_education", 0.97, 0.98, 0,
					"风险词位于教育或科普关系中，同句存在明确的危害、防范或救助结果"}
			}
			if relation.Negated {
				return localIntentProfile{"rejection", 0.94, 0.97, 0, "否定词作用于具体风险关系"}
			}
		}
		return localIntentProfile{"reporting", 0.92, 0.94, 0, "风险关系位于引用或转述范围"}
	}
	compact := text.Compact
	if containsAnyNormalized(compact, cfg.SemanticRules.RelationVocabulary.QuestionMarkers) {
		return localIntentProfile{"question", 0.84, 0.94, 0, "未形成实施关系，评论为询问表达"}
	}
	if containsAnyNormalized(compact, cfg.SemanticRules.Contexts.TechnicalMarkers) {
		return localIntentProfile{"technical", 0.8, 0.86, 0, "未形成实施关系，评论为技术讨论"}
	}
	return localIntentProfile{"unknown", 0.72, 0.08, 0, "未形成足够完整的关系"}
}

// relationsAreUntargetedAbuseOpinions 判断 relations 是否全部为未达到严重攻击的内容评价型辱骂。
// 第二个配置参数保留接口一致性；返回 true 表示可按内容批评而非对人攻击处理。
func relationsAreUntargetedAbuseOpinions(relations []SemanticRelation,
	_ appconfig.CommentModerationConfig,
) bool {
	if len(relations) == 0 {
		return false
	}
	for _, relation := range relations {
		if relation.Category != "abuse" || relation.Action == "严重攻击" {
			return false
		}
	}
	return true
}

// absInt 返回整数 value 的绝对值。
func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

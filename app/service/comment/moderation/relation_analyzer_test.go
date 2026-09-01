package moderation

import (
	"strings"
	"testing"

	appconfig "meta-api/config"
)

func TestSemanticRelationsKeepNegationAndQuotationScoped(t *testing.T) {
	cfg := relationTestConfig()

	negated := analyzeSemanticRelations(Normalize("我不出售账号，只讨论数据库账号"), nil, nil, cfg)
	relation, ok := findSemanticRelation(negated, "出售", "账号")
	if !ok || !relation.Negated || relation.Quoted || relation.Reported {
		t.Fatalf("negated relation = %+v, found = %v", relation, ok)
	}

	reported := analyzeSemanticRelations(Normalize("案例中写“低价出售账号”，这是违规样例"), nil, nil, cfg)
	relation, ok = findSemanticRelation(reported, "出售", "账号")
	if !ok || relation.Negated || !relation.Quoted || !relation.Reported || !relationIsCounterEvidence(relation) {
		t.Fatalf("reported relation = %+v, found = %v", relation, ok)
	}
}

func TestSemanticRelationsExtractActorActionObjectAndResult(t *testing.T) {
	cfg := relationTestConfig()
	relations := analyzeSemanticRelations(Normalize("我这边低价出售账号，保证长期稳定"), nil, nil, cfg)
	relation, ok := findSemanticRelation(relations, "出售", "账号")
	if !ok || relation.Subject != "我这边" || relation.Result != "保证长期稳定" ||
		relation.Negated || relation.Quoted || relation.Reported {
		t.Fatalf("actionable relation = %+v, found = %v", relation, ok)
	}
	profile := relationIntentProfile(relations, Normalize("我这边低价出售账号，保证长期稳定"), cfg)
	if profile.intent != "actionable" || profile.benignProbability > 0.1 {
		t.Fatalf("relation profile = %+v", profile)
	}
}

func TestSemanticRelationsDoNotJoinAcrossClauses(t *testing.T) {
	cfg := relationTestConfig()
	relations := analyzeSemanticRelations(Normalize("前一段介绍数据库账号。下一段讨论商品出售流程。"), nil, nil, cfg)
	if relation, ok := findSemanticRelation(relations, "出售", "账号"); ok {
		t.Fatalf("cross-clause relation was fabricated: %+v", relation)
	}
}

func TestSemanticRelationsDoNotTreatRhetoricalQuestionAsNegation(t *testing.T) {
	cfg := relationTestConfig()
	cfg.SemanticRules.AbusePolicy.SevereMarkers = []string{"脑子是不是有坑"}
	relations := analyzeSemanticRelations(
		Normalize("这作者脑子是不是有🕳️，这都能写错"),
		[]RewriteCandidate{{
			Text: "脑子是不是有坑", Observed: "🕳️", Category: "abuse",
			Role: CandidateRoleConcept, Method: "emoji_annotation", Confidence: 0.82, Clause: 1,
		}},
		nil,
		cfg,
	)
	relation, ok := findSemanticRelation(relations, "严重攻击", "作者")
	if !ok || relation.Negated || relation.Quoted {
		t.Fatalf("rhetorical abuse relation = %+v, found = %v", relation, ok)
	}
}

func TestHarmfulValueRelationsRequireTargetDirectiveAndDangerousAction(t *testing.T) {
	cfg := harmfulRelationTestConfig()
	tests := []struct {
		text    string
		subtype string
		result  string
	}{
		{"你怎么不去跳楼", relationSubtypeSelfHarmEncouragement, "跳楼"},
		{"你去喝敌敌畏吧", relationSubtypeDangerousBehaviorEncouragement, "喝敌敌畏"},
		{"他有本事就去割腕", relationSubtypeSelfHarmEncouragement, "割腕"},
		{"你去死吧", relationSubtypeDeathWish, "去死"},
		{"我希望他去死", relationSubtypeDeathWish, "去死"},
	}
	for _, test := range tests {
		t.Run(test.text, func(t *testing.T) {
			relations := analyzeSemanticRelations(Normalize(test.text), nil, nil, cfg)
			relation, ok := findRelationSubtype(relations, test.subtype)
			if !ok || relation.Category != "harmful_value" || relation.Subject != "评论者" ||
				relation.Object == "" || relation.Result != test.result || relationIsCounterEvidence(relation) {
				t.Fatalf("harmful relation = %+v, found = %v; all = %+v", relation, ok, relations)
			}
		})
	}
	if relation, ok := findRelationSubtype(
		analyzeSemanticRelations(Normalize("他去死了"), nil, nil, cfg), relationSubtypeDeathWish,
	); ok {
		t.Fatalf("reported death was misclassified as a death wish: %+v", relation)
	}
}

func TestHarmfulValueRelationsKeepPreventionAndSelfExpressionDistinct(t *testing.T) {
	cfg := harmfulRelationTestConfig()

	prevented := analyzeSemanticRelations(Normalize("我劝你不要跳楼，有问题及时求助"), nil, nil, cfg)
	prevention, ok := findRelationSubtype(prevented, relationSubtypeRiskPrevention)
	if !ok || !prevention.Negated || !relationIsCounterEvidence(prevention) {
		t.Fatalf("prevention relation = %+v, found = %v; all = %+v", prevention, ok, prevented)
	}

	expression := analyzeSemanticRelations(Normalize("我最近压力很大，开始想跳楼"), nil, nil, cfg)
	selfHarm, ok := findRelationSubtype(expression, relationSubtypeSelfHarmExpression)
	if !ok || selfHarm.Confidence > 0.62 || relationIsCounterEvidence(selfHarm) {
		t.Fatalf("self-harm expression = %+v, found = %v; all = %+v", selfHarm, ok, expression)
	}

	directed := analyzeSemanticRelations(Normalize("我想让你去跳楼"), nil, nil, cfg)
	if relation, ok := findRelationSubtype(directed, relationSubtypeSelfHarmExpression); ok {
		t.Fatalf("directed incitement was misclassified as self expression: %+v", relation)
	}
	incitement, ok := findRelationSubtype(directed, relationSubtypeSelfHarmEncouragement)
	if !ok || incitement.Object != "你" {
		t.Fatalf("directed incitement = %+v, found = %v; all = %+v", incitement, ok, directed)
	}

	reported := analyzeSemanticRelations(Normalize("我听说他想跳楼"), nil, nil, cfg)
	if relation, ok := findRelationSubtype(reported, relationSubtypeSelfHarmExpression); ok {
		t.Fatalf("third-person report was misclassified as self expression: %+v", relation)
	}
}

func TestHarmfulValueRelationsUseEducationAsCategorySpecificCounterEvidence(t *testing.T) {
	cfg := harmfulRelationTestConfig()
	evidence := []Evidence{
		{Source: SourceLexicon, Category: "sensitive", Polarity: "positive", Value: "法轮功", Clause: 1},
		{Source: SourceContext, Category: "political", Polarity: "positive", Value: "法轮功+传播", Clause: 1},
	}
	relations := analyzeSemanticRelations(
		Normalize("今天上课老师科普法轮功的知识，让我们了解其危害"), nil, evidence, cfg,
	)
	for _, category := range []string{"sensitive", "political"} {
		relation, ok := findRelationCategorySubtype(relations, category, relationSubtypeRiskEducation)
		if !ok || !relation.Reported || relation.Subject != "老师" || relation.Action != "科普" ||
			relation.Object != "法轮功" || relation.Result != "危害" {
			t.Fatalf("education relation for %s = %+v, found = %v; all = %+v", category, relation, ok, relations)
		}
	}

	conflicting := analyzeSemanticRelations(
		Normalize("老师科普法轮功危害不大，建议加入"), nil, evidence, cfg,
	)
	if relation, ok := findRelationSubtype(conflicting, relationSubtypeRiskEducation); ok {
		t.Fatalf("promotional expression received education counter-evidence: %+v", relation)
	}
}

func TestHarmfulValueRelationsAcceptLocallyDerivedVariantCandidate(t *testing.T) {
	cfg := harmfulRelationTestConfig()
	relations := analyzeSemanticRelations(Normalize("你怎么不去跳搂"), []RewriteCandidate{{
		Text: "跳楼", Observed: "跳搂", Category: "harmful_value", Role: CandidateRoleConcept,
		Method: "pinyin_homophone", Confidence: 0.92, Clause: 1,
	}}, nil, cfg)
	relation, ok := findRelationSubtype(relations, relationSubtypeSelfHarmEncouragement)
	if !ok || relation.Result != "跳楼" || !strings.Contains(relation.Evidence, "跳搂→跳楼") {
		t.Fatalf("variant harmful relation = %+v, found = %v; all = %+v", relation, ok, relations)
	}
}

func TestSemanticRelationsModelRiskStanceAsEvaluation(t *testing.T) {
	cfg := relationTestConfig()
	cfg.CombinationRules = append(cfg.CombinationRules, appconfig.CommentModerationCombinationRuleConfig{
		ID: "academic_service", Category: "spam_fraud",
		Subjects: []string{"毕业设计"}, Predicates: []string{"代做"},
	})
	evidence := []Evidence{
		{Source: SourceLexicon, Category: "spam_fraud", Polarity: "positive", Value: "代做", Clause: 1},
	}
	relations := analyzeSemanticRelations(Normalize("毕业设计代做属于非法产业"), nil, evidence, cfg)
	relation, ok := findRelationSubtype(relations, relationSubtypeStanceEvaluation)
	if !ok || relation.Type != RelationTypeEvaluation || relation.Action != "评价" ||
		relation.Object != "毕业设计代做" || relation.Predicate != "属于" || relation.Result != "非法产业" ||
		relation.Stance != RelationStanceCondemnation || !relationIsCounterEvidence(relation) || relation.Inferred {
		t.Fatalf("stance evaluation = %+v, found = %v; all = %+v", relation, ok, relations)
	}
	profile := relationIntentProfile(relations, Normalize("毕业设计代做属于非法产业"), cfg)
	if profile.intent != "risk_evaluation" || profile.benignProbability < 0.95 {
		t.Fatalf("stance evaluation profile = %+v", profile)
	}
}

func TestSemanticRelationsKeepStanceEvaluationBoundaries(t *testing.T) {
	cfg := relationTestConfig()
	evidence := []Evidence{
		{Source: SourceContext, Category: "spam_fraud", Polarity: "positive", Value: "毕业设计+代做", Clause: 1},
	}
	for _, content := range []string{
		"毕业设计代做不属于非法产业",
		"毕业设计代做是不是违法行为",
		"毕业设计代做属于非法产业不过我仍然接单",
	} {
		relations := analyzeSemanticRelations(Normalize(content), nil, evidence, cfg)
		if relation, ok := findRelationSubtype(relations, relationSubtypeStanceEvaluation); ok {
			t.Errorf("%q received stance counter-evidence: %+v; all = %+v", content, relation, relations)
		}
	}
}

func TestSemanticRelationsRecognizeCategoryIndependentGovernanceStance(t *testing.T) {
	cfg := relationTestConfig()
	evidence := []Evidence{
		{Source: SourceContext, Category: "spam_fraud", Polarity: "positive", Value: "论文+代写", Clause: 1},
	}
	relations := analyzeSemanticRelations(Normalize("论文代写应当严厉打击"), nil, evidence, cfg)
	relation, ok := findRelationSubtype(relations, relationSubtypeStanceEvaluation)
	if !ok || relation.Object != "论文代写" || relation.Predicate != "应当" ||
		relation.Result != "严厉打击" || relation.Stance != RelationStanceCondemnation {
		t.Fatalf("governance stance = %+v, found = %v; all = %+v", relation, ok, relations)
	}
}

func TestSemanticRelationsRecognizeDemonstrativeRiskEvaluation(t *testing.T) {
	cfg := relationTestConfig()
	evidence := []Evidence{
		{Source: SourceContext, Category: "spam_fraud", Polarity: "positive", Value: "资源+私聊", Clause: 1},
		{Source: SourceContext, Category: "spam_fraud", Polarity: "positive", Value: "资源+发", Clause: 1},
	}
	relations := analyzeSemanticRelations(Normalize("私聊发资源这种套路不可信，建议直接举报"), nil, evidence, cfg)
	relation, ok := findRelationSubtype(relations, relationSubtypeStanceEvaluation)
	if !ok || relation.Object != "私聊发资源" || relation.Predicate != "这种" ||
		relation.Result != "套路" || relation.Stance != RelationStanceWarning {
		t.Fatalf("demonstrative stance = %+v, found = %v; all = %+v", relation, ok, relations)
	}

	unsafe := analyzeSemanticRelations(Normalize("私聊发资源这种套路很好用"), nil, evidence, cfg)
	if relation, ok := findRelationSubtype(unsafe, relationSubtypeStanceEvaluation); ok {
		t.Fatalf("positive endorsement received stance counter-evidence: %+v", relation)
	}
}

func relationTestConfig() appconfig.CommentModerationConfig {
	cfg := appconfig.CommentModerationConfig{
		CombinationRules: []appconfig.CommentModerationCombinationRuleConfig{
			{
				ID:         "commercial_spam",
				Category:   "spam_fraud",
				Subjects:   []string{"账号"},
				Predicates: []string{"出售"},
			},
		},
		SemanticRules: appconfig.CommentModerationSemanticRulesConfig{
			Contexts: appconfig.CommentModerationSemanticContextConfig{
				ReportingMarkers:  []string{"案例", "样例", "引用", "写"},
				RejectionMarkers:  []string{"不", "禁止", "拒绝"},
				ActionableMarkers: []string{"出售", "提供", "加我"},
			},
		},
	}
	return compileConfig(cfg)
}

func harmfulRelationTestConfig() appconfig.CommentModerationConfig {
	cfg := relationTestConfig()
	cfg.SemanticRules.HarmfulValuePolicy = appconfig.CommentModerationHarmfulValuePolicyConfig{
		SelfHarmActions:     []string{"跳楼", "割腕", "上吊", "服毒", "自杀"},
		DeathWishActions:    []string{"去死", "别活", "不配活"},
		DangerousActions:    []string{"卧轨", "摸高压线"},
		DangerousSubstances: []string{"敌敌畏", "百草枯", "农药", "鼠药"},
		IngestionActions:    []string{"喝", "吞", "吃", "服", "服用", "灌"},
		IncitementMarkers:   []string{"怎么不", "有本事就", "敢不敢", "不如", "就该", "赶紧", "快去", "最好", "希望", "巴不得", "建议", "应该", "去"},
		IncitementSuffixes:  []string{"吧", "算了", "得了"},
		IdeationMarkers:     []string{"想", "想要", "打算", "准备", "不想活", "活不下去"},
		PreventionMarkers:   []string{"不要", "别", "请勿", "禁止", "防止", "阻止", "劝阻", "救下", "担心", "求助", "就医"},
		EducationActors:     []string{"老师", "学校", "课堂", "上课", "课程", "教材", "文章", "新闻", "警方", "医生", "平台"},
		EducationActions:    []string{"科普", "讲解", "介绍", "了解", "提醒", "警示", "分析", "批判", "揭露", "通报"},
		CriticalOutcomes:    []string{"危害", "风险", "有毒", "危险", "违法", "防范", "远离", "抵制", "举报", "打击", "救助", "求助", "就医"},
	}
	return compileConfig(cfg)
}

func findRelationSubtype(relations []SemanticRelation, subtype string) (SemanticRelation, bool) {
	for _, relation := range relations {
		if relation.Subtype == subtype {
			return relation, true
		}
	}
	return SemanticRelation{}, false
}

func findRelationCategorySubtype(relations []SemanticRelation, category, subtype string) (SemanticRelation, bool) {
	for _, relation := range relations {
		if relation.Category == category && relation.Subtype == subtype {
			return relation, true
		}
	}
	return SemanticRelation{}, false
}

func findSemanticRelation(relations []SemanticRelation, action, object string) (SemanticRelation, bool) {
	for _, relation := range relations {
		if relation.Action == action && relation.Object == object {
			return relation, true
		}
	}
	return SemanticRelation{}, false
}

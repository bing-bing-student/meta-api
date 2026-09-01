package moderation

import (
	"context"
	"testing"

	"go.uber.org/zap"

	commentModel "meta-api/app/model/comment"
	appconfig "meta-api/config"
)

func TestLocalContextAnalyzerRestoresRiskVariants(t *testing.T) {
	analyzer := NewLocalContextAnalyzer(zap.NewNop())
	cfg := localContextTestConfig()

	tests := []struct {
		name      string
		content   string
		observed  string
		canonical string
		method    string
	}{
		{name: "pinyin initials", content: "你真是个sb", observed: "sb", canonical: "傻逼", method: "pinyin_initials"},
		{name: "full pinyin", content: "dijiachushou", observed: "dijiachushou", canonical: "低价出售", method: "pinyin_full"},
		{name: "full pinyin with suffix", content: "naoziyouwentiba", observed: "naoziyouwenti", canonical: "脑子有问题", method: "pinyin_full"},
		{name: "mixed Han and pinyin", content: "编程zi源包", observed: "编程zi源包", canonical: "编程资源包", method: "pinyin_full"},
		{name: "mixed Han and initials", content: "内部qd", observed: "内部qd", canonical: "内部渠道", method: "pinyin_initials"},
		{name: "long full pinyin", content: "chengrenwangzhanhuiyuan", observed: "chengrenwangzhanhuiyuan", canonical: "成人网站会员", method: "pinyin_full"},
		{name: "homophone", content: "你个沙壁", observed: "沙壁", canonical: "傻逼", method: "pinyin_homophone"},
		{name: "contextual initials", content: "ltp 是什么意思", observed: "ltp", canonical: "恋童癖", method: "pinyin_initials"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assessment, err := analyzer.Analyze(context.Background(), ContextInput{
				Request: Request{Content: test.content},
				Text:    Normalize(test.content),
			}, cfg)
			if err != nil {
				t.Fatalf("Analyze() error = %v", err)
			}
			candidate, ok := findRewriteCandidate(assessment.Candidates, test.observed, test.canonical)
			if !ok || candidate.Method != test.method {
				t.Fatalf("candidates = %+v, want %s -> %s via %s",
					assessment.Candidates, test.observed, test.canonical, test.method)
			}
		})
	}
}

func TestLocalContextAnalyzerUsesIntentAsCounterEvidence(t *testing.T) {
	analyzer := NewLocalContextAnalyzer(zap.NewNop())
	cfg := localContextTestConfig()

	question, err := analyzer.Analyze(context.Background(), ContextInput{
		Request: Request{Content: "ltp 是什么意思"},
		Text:    Normalize("ltp 是什么意思"),
	}, cfg)
	if err != nil {
		t.Fatalf("question Analyze() error = %v", err)
	}
	if question.Intent != "question" || question.BenignProbability < 0.9 {
		t.Fatalf("question assessment = %+v, want strong benign question context", question)
	}

	actionable, err := analyzer.Analyze(context.Background(), ContextInput{
		Request: Request{Content: "分享ltp资源，加我"},
		Text:    Normalize("分享ltp资源，加我"),
	}, cfg)
	if err != nil {
		t.Fatalf("actionable Analyze() error = %v", err)
	}
	if actionable.Intent != "actionable" || actionable.BenignProbability > 0.1 ||
		actionable.CategoryProbabilities["minor"] < question.CategoryProbabilities["minor"] {
		t.Fatalf("actionable assessment = %+v, question = %+v", actionable, question)
	}
	questionDecision := fuseEvidence(question.Evidence, question, cfg.DecisionEngine)
	actionableDecision := fuseEvidence(actionable.Evidence, actionable, cfg.DecisionEngine)
	if questionDecision.Status != commentModel.StatusApproved ||
		actionableDecision.Status != commentModel.StatusRejected ||
		actionableDecision.RiskProbability <= questionDecision.RiskProbability {
		t.Fatalf("question decision = %+v, actionable decision = %+v",
			questionDecision, actionableDecision)
	}
}

func TestLocalContextAnalyzerDoesNotGeneralizeNonRiskHomophones(t *testing.T) {
	analyzer := NewLocalContextAnalyzer(zap.NewNop())
	assessment, err := analyzer.Analyze(context.Background(), ContextInput{
		Request: Request{Content: "这对角色结对甜"},
		Text:    Normalize("这对角色结对甜"),
	}, localContextTestConfig())
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if len(assessment.Candidates) != 0 {
		t.Fatalf("non-risk text produced moderation candidates: %+v", assessment.Candidates)
	}
}

func TestLocalContextAnalyzerUsesSurroundingCanonicalTerm(t *testing.T) {
	analyzer := NewLocalContextAnalyzer(zap.NewNop())
	assessment, err := analyzer.Analyze(context.Background(), ContextInput{
		Request: Request{
			Content:       "这里的 ltp 是缩写",
			ParentContent: "上一条正在讨论如何识别恋童癖相关表达",
		},
		Text: Normalize("这里的 ltp 是缩写"),
	}, localContextTestConfig())
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	candidate, ok := findRewriteCandidate(assessment.Candidates, "ltp", "恋童癖")
	if !ok || candidate.Confidence < 0.9 ||
		candidate.Rationale != "本地拼音索引候选，并由文章或回复上下文中的规范词印证" {
		t.Fatalf("context-supported candidate = %+v, found = %v", candidate, ok)
	}
}

func TestLocalContextAnalyzerRestoresCombinationPredicateHomophones(t *testing.T) {
	analyzer := NewLocalContextAnalyzer(zap.NewNop())
	cfg := localCombinationVariantTestConfig()

	for _, observed := range []string{"丝聊", "司聊", "撕聊"} {
		t.Run(observed, func(t *testing.T) {
			content := "有资源的" + observed + "我"
			assessment, err := analyzer.Analyze(context.Background(), ContextInput{
				Request: Request{Content: content},
				Text:    Normalize(content),
			}, cfg)
			if err != nil {
				t.Fatalf("Analyze() error = %v", err)
			}
			candidate, ok := findRewriteCandidate(assessment.Candidates, observed, "私聊")
			if !ok || candidate.Role != CandidateRolePredicate || candidate.Method != "pinyin_homophone" {
				t.Fatalf("predicate candidate = %+v, found = %v", candidate, ok)
			}
			relation, ok := findSemanticRelation(assessment.Relations, "私聊", "资源")
			if !ok || relation.Negated || relation.Quoted || relation.Reported ||
				assessment.Intent != "actionable" || assessment.CategoryProbabilities["spam_fraud"] < 0.9 {
				t.Fatalf("assessment = %+v, relation = %+v, found = %v", assessment, relation, ok)
			}
		})
	}
}

func TestLocalContextAnalyzerRequiresACompleteRelationForPredicateVariant(t *testing.T) {
	analyzer := NewLocalContextAnalyzer(zap.NewNop())
	cfg := localCombinationVariantTestConfig()

	assessment, err := analyzer.Analyze(context.Background(), ContextInput{
		Request: Request{Content: "不要丝聊我，请公开回复"},
		Text:    Normalize("不要丝聊我，请公开回复"),
	}, cfg)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if _, ok := findRewriteCandidate(assessment.Candidates, "丝聊", "私聊"); !ok {
		t.Fatalf("missing diagnostic predicate candidate: %+v", assessment.Candidates)
	}
	if relation, ok := findSemanticRelation(assessment.Relations, "私聊", "资源"); ok {
		t.Fatalf("standalone predicate variant formed a risk relation: %+v", relation)
	}
	if probability := assessment.CategoryProbabilities["spam_fraud"]; probability > 0.2 {
		t.Fatalf("standalone predicate variant risk = %.4f, want <= 0.2; evidence = %+v",
			probability, assessment.Evidence)
	}
}

func TestLocalContextAnalyzerKeepsNegationOnPredicateVariant(t *testing.T) {
	analyzer := NewLocalContextAnalyzer(zap.NewNop())
	cfg := localCombinationVariantTestConfig()
	assessment, err := analyzer.Analyze(context.Background(), ContextInput{
		Request: Request{Content: "有资源但不要丝聊我"},
		Text:    Normalize("有资源但不要丝聊我"),
	}, cfg)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	relation, ok := findSemanticRelation(assessment.Relations, "私聊", "资源")
	if !ok || !relation.Negated || assessment.Intent != "rejection" ||
		assessment.BenignProbability < 0.9 {
		t.Fatalf("negated assessment = %+v, relation = %+v, found = %v", assessment, relation, ok)
	}
}

func TestLocalContextAnalyzerTreatsFraudJudgmentAsRiskEvaluation(t *testing.T) {
	analyzer := NewLocalContextAnalyzer(zap.NewNop())
	cfg := localCombinationVariantTestConfig()
	cfg.CombinationRules[0].Predicates = append(cfg.CombinationRules[0].Predicates, "领取")
	content := "评论区那种私聊领取资源的基本都是诈骗"
	assessment, err := analyzer.Analyze(context.Background(), ContextInput{
		Request: Request{Content: content},
		Text:    Normalize(content),
		Evidence: []Evidence{
			{Source: SourceContext, Category: "spam_fraud", Polarity: "positive", Confidence: 0.66,
				Value: "资源+私聊", Clause: 1},
			{Source: SourceContext, Category: "spam_fraud", Polarity: "positive", Confidence: 0.66,
				Value: "资源+领取", Clause: 1},
		},
	}, cfg)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if assessment.Intent != "risk_evaluation" || assessment.BenignProbability < 0.95 {
		t.Fatalf("risk evaluation assessment = %+v", assessment)
	}
	foundCounter := false
	for _, relation := range assessment.Relations {
		if relation.Category == "spam_fraud" && relation.Subtype == relationSubtypeStanceEvaluation &&
			relation.Type == RelationTypeEvaluation && relation.Predicate == "基本都是" &&
			relation.Stance == RelationStanceWarning && relation.Result == "诈骗" {
			foundCounter = true
		}
		if relation.Category == "spam_fraud" && relationIsActionableRisk(relation) {
			t.Fatalf("risk evaluation retained an actionable relation: %+v", relation)
		}
	}
	if !foundCounter {
		t.Fatalf("missing risk education relation: %+v", assessment.Relations)
	}
}

func TestLocalContextAnalyzerKeepsPromotionAfterFraudWarningActionable(t *testing.T) {
	analyzer := NewLocalContextAnalyzer(zap.NewNop())
	cfg := localCombinationVariantTestConfig()
	content := "私聊领取资源的基本都是诈骗，但我这里有真的资源，需要的私聊我"
	assessment, err := analyzer.Analyze(context.Background(), ContextInput{
		Request: Request{Content: content},
		Text:    Normalize(content),
	}, cfg)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if assessment.Intent != "actionable" || assessment.BenignProbability > 0.1 {
		t.Fatalf("promotion after fraud warning was treated as benign: %+v", assessment)
	}
}

func TestLocalContextAnalyzerDoesNotReuseOverlappingTextAsAnAction(t *testing.T) {
	analyzer := NewLocalContextAnalyzer(zap.NewNop())
	cfg := localCombinationVariantTestConfig()
	cfg.DecisionEngine.ContextAnalysis.RiskConcepts = map[string][]string{
		"spam_fraud": {"不要米"},
	}
	assessment, err := analyzer.Analyze(context.Background(), ContextInput{
		Request: Request{Content: "不要私聊我，请公开回复"},
		Text:    Normalize("不要私聊我，请公开回复"),
	}, cfg)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if probability := assessment.CategoryProbabilities["spam_fraud"]; probability > 0.2 {
		t.Fatalf("overlapping candidate risk = %.4f, want <= 0.2; candidates = %+v relations = %+v evidence = %+v",
			probability, assessment.Candidates, assessment.Relations, assessment.Evidence)
	}
}

func localCombinationVariantTestConfig() appconfig.CommentModerationConfig {
	cfg := appconfig.CommentModerationConfig{
		CombinationRules: []appconfig.CommentModerationCombinationRuleConfig{
			{
				ID:         "resource_diversion",
				Category:   "spam_fraud",
				Level:      LevelReview,
				Subjects:   []string{"资源"},
				Predicates: []string{"私聊"},
			},
		},
		DecisionEngine: appconfig.CommentModerationDecisionEngineConfig{
			ContextAnalysis: appconfig.CommentModerationContextAnalysisConfig{MaxCandidates: 32},
		},
		SemanticRules: appconfig.CommentModerationSemanticRulesConfig{
			Contexts: appconfig.CommentModerationSemanticContextConfig{
				ActionableMarkers: []string{"私聊", "提供", "出售"},
				ReportingMarkers:  []string{"案例", "样例", "引用"},
				RejectionMarkers:  []string{"不要", "禁止", "拒绝"},
			},
		},
	}
	ApplyDefaults(&cfg)
	return compileConfig(cfg)
}

func localContextTestConfig() appconfig.CommentModerationConfig {
	cfg := appconfig.CommentModerationConfig{
		DecisionEngine: appconfig.CommentModerationDecisionEngineConfig{
			ContextAnalysis: appconfig.CommentModerationContextAnalysisConfig{
				MaxCandidates: 32,
				RiskConcepts: map[string][]string{
					"abuse":      {"脑子有问题"},
					"minor":      {"恋童癖"},
					"sexual":     {"成人网站会员"},
					"spam_fraud": {"低价出售", "编程资源包", "内部渠道"},
				},
			},
		},
		SemanticRules: appconfig.CommentModerationSemanticRulesConfig{
			Contexts: appconfig.CommentModerationSemanticContextConfig{
				ActionableMarkers: []string{"分享", "加我", "提供", "出售"},
				ReportingMarkers:  []string{"举报", "案例", "引用"},
				RejectionMarkers:  []string{"反对", "禁止", "拒绝"},
				TechnicalMarkers:  []string{"安全研究", "技术分析"},
			},
		},
	}
	ApplyDefaults(&cfg)
	return cfg
}

func findRewriteCandidate(candidates []RewriteCandidate, observed, canonical string) (RewriteCandidate, bool) {
	for _, candidate := range candidates {
		if candidate.Observed == observed && candidate.Text == canonical {
			return candidate, true
		}
	}
	return RewriteCandidate{}, false
}

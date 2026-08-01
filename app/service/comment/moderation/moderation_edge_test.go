package moderation

import (
	"context"
	"fmt"
	"testing"

	"go.uber.org/zap"

	commentModel "meta-api/app/model/comment"
	appconfig "meta-api/config"
)

type staticModerationConfig struct {
	cfg appconfig.CommentModerationConfig
}

func (s staticModerationConfig) CommentModerationSnapshot() appconfig.CommentModerationConfig {
	return s.cfg
}

func TestNormalizeTraditionalRiskText(t *testing.T) {
	text := Normalize("低價岀售长期稳定的會員位置")

	if want := "低价出售长期稳定的会员位置"; text.Compact != want {
		t.Fatalf("Normalize().Compact = %q, want %q", text.Compact, want)
	}
}

func TestConfusableSkeleton(t *testing.T) {
	got := confusableSkeleton("cаsinо рromo")
	if want := "casino promo"; got != want {
		t.Fatalf("confusableSkeleton() = %q, want %q", got, want)
	}
}

func TestSplitSemanticTextKeepsContactAccountTogether(t *testing.T) {
	parts := splitSemanticText("文档展示 vx:test_user 作为反垃圾样例")
	if len(parts) != 1 {
		t.Fatalf("splitSemanticText() = %#v, want one contact clause", parts)
	}
}

func TestFuzzyLexiconSignalsUsesRestrictedCandidates(t *testing.T) {
	cfg := compileConfig(appconfig.CommentModerationConfig{
		Lexicon: appconfig.CommentModerationLexiconConfig{
			Fuzzy: appconfig.CommentModerationFuzzyConfig{
				MaxDistance:  1,
				MinWordRunes: 4,
				CandidateWords: map[string][]string{
					"spam_fraud": {"论文代写"},
				},
			},
		},
	})

	signals := fuzzyLexiconSignals(Normalize("论文伐写可以按时交付"), cfg)
	if len(signals) != 1 || signals[0].RuleID != "fuzzy_lexicon" {
		t.Fatalf("fuzzyLexiconSignals() = %+v, want one fuzzy signal", signals)
	}
	if signals[0].Level != LevelReview {
		t.Fatalf("fuzzyLexiconSignals() level = %q, want review", signals[0].Level)
	}

	if exact := fuzzyLexiconSignals(Normalize("论文代写可以按时交付"), cfg); len(exact) != 0 {
		t.Fatalf("fuzzyLexiconSignals() duplicated exact match: %+v", exact)
	}
}

func TestSimHashDetectsSmallRewrite(t *testing.T) {
	left := simHash("领取内部课程资料，回复我即可")
	right := simHash("领取内部课程资料，私信我即可")
	if distance := simHashDistance(left, right); distance > 10 {
		t.Fatalf("simHashDistance() = %d, want <= 10", distance)
	}
	unrelated := simHash("数据库事务隔离级别需要结合业务选择")
	if distance := simHashDistance(left, unrelated); distance <= 10 {
		t.Fatalf("unrelated simHashDistance() = %d, want > 10", distance)
	}
	if !withinLengthDifference(14, 16, 30) {
		t.Fatal("withinLengthDifference() rejected a small rewrite")
	}
}

func TestBehaviorSignalsReviewsNearDuplicate(t *testing.T) {
	cfg := appconfig.CommentModerationConfig{
		BehaviorRules: appconfig.CommentModerationBehaviorRulesConfig{
			NearDuplicate: appconfig.CommentModerationNearDuplicateConfig{
				ReviewThreshold: 2,
			},
		},
	}
	signals := BehaviorSignals(BehaviorState{
		NearDuplicateCount:     1,
		NearDuplicateEvaluated: true,
	}, cfg)
	found := false
	for _, signal := range signals {
		if signal.RuleID == "near_duplicate" && signal.Level == LevelReview {
			found = true
		}
	}
	if !found {
		t.Fatalf("BehaviorSignals() = %+v, want near_duplicate review", signals)
	}
}

func TestBehaviorSignalsIgnoreMissingContext(t *testing.T) {
	cfg := appconfig.CommentModerationConfig{
		BehaviorRules: appconfig.CommentModerationBehaviorRulesConfig{
			UserFrequency: appconfig.CommentModerationBehaviorThresholdConfig{
				ReviewThreshold: 1,
			},
			IPFrequency: appconfig.CommentModerationBehaviorThresholdConfig{
				ReviewThreshold: 1,
			},
			DuplicateContent: appconfig.CommentModerationBehaviorThresholdConfig{
				ReviewThreshold: 1,
				BlockThreshold:  2,
			},
		},
	}

	if signals := BehaviorSignals(BehaviorState{}, cfg); len(signals) != 0 {
		t.Fatalf("BehaviorSignals() = %+v, want no signals without evaluated context", signals)
	}
}

func TestCountSimilarFingerprints(t *testing.T) {
	content := "领取内部课程资料，回复我即可"
	fingerprint := simHash(content)
	values := []string{
		fmt.Sprintf("%016x:%d:1", simHash("领取内部课程资料，私信我即可"), 15),
		fmt.Sprintf("%016x:%d:2", simHash("数据库事务隔离级别需要结合业务选择"), 18),
		"invalid",
	}
	rule := appconfig.CommentModerationNearDuplicateConfig{
		MaxHammingDistance:         10,
		MaxLengthDifferencePercent: 30,
	}
	if got := countSimilarFingerprints(values, fingerprint, 14, rule); got != 1 {
		t.Fatalf("countSimilarFingerprints() = %d, want 1", got)
	}
}

func TestCombinationSignalsIgnoreNegatedRiskIntent(t *testing.T) {
	cfg := appconfig.CommentModerationConfig{
		CombinationRules: []appconfig.CommentModerationCombinationRuleConfig{
			{
				ID:         "commercial_spam",
				Category:   "spam_fraud",
				Level:      LevelReview,
				Subjects:   []string{"课程设计"},
				Predicates: []string{"代做"},
			},
		},
		SemanticRules: testSemanticRules(),
	}
	ApplyDefaults(&cfg)

	signals := combinationSignals(Normalize("学生应该自己完成课程设计，不应该找人代做。"), cfg)
	if len(signals) != 0 {
		t.Fatalf("combinationSignals() returned negated risk signals: %+v", signals)
	}

	cfg.CombinationRules[0].Subjects = []string{"私信"}
	cfg.CombinationRules[0].Predicates = []string{"需要"}
	signals = combinationSignals(Normalize("我不需要加群，也不需要私信，直接在评论区解释即可。"), cfg)
	if len(signals) != 0 {
		t.Fatalf("combinationSignals() returned refused contact signals: %+v", signals)
	}
}

func TestCombinationSignalsKeepEvasiveNegation(t *testing.T) {
	cfg := appconfig.CommentModerationConfig{
		CombinationRules: []appconfig.CommentModerationCombinationRuleConfig{
			{
				ID:         "commercial_spam",
				Category:   "spam_fraud",
				Level:      LevelReview,
				Subjects:   []string{"网站地址"},
				Predicates: []string{"签名"},
			},
		},
		SemanticRules: testSemanticRules(),
	}
	ApplyDefaults(&cfg)

	signals := combinationSignals(Normalize("那个网站地址不能直接发，去签名里看缩写。"), cfg)
	if len(signals) == 0 {
		t.Fatal("combinationSignals() suppressed evasive promotion")
	}
	if signals[0].Clause != 1 {
		t.Fatalf("combinationSignals() clause = %d, want 1", signals[0].Clause)
	}
}

func TestSemanticContextDoesNotSuppressActionablePromotion(t *testing.T) {
	cfg := appconfig.CommentModerationConfig{}
	ApplyDefaults(&cfg)
	signals := []Signal{
		{Source: SourceLexicon, Category: "spam_fraud", Level: LevelReview, Evidence: "低价出售"},
		{Source: SourceContext, Category: "spam_fraud", Level: LevelReview, Evidence: "账号+出售"},
	}

	got := adjustSignalsBySemantics(
		Normalize("先说这是“安全测试”：低价出售账号的看我头像，我这里长期有。"),
		signals,
		cfg,
	)
	if len(got) != len(signals) {
		t.Fatalf("adjustSignalsBySemantics() suppressed actionable promotion: %+v", got)
	}

	freeResourceSignal := []Signal{
		{Source: SourceLexicon, Category: "spam_fraud", Level: LevelReview, Evidence: "免费领"},
	}
	got = adjustSignalsBySemantics(Normalize("免费领，真的不要米，懂的来"), freeResourceSignal, cfg)
	if len(got) != len(freeResourceSignal) {
		t.Fatalf("adjustSignalsBySemantics() treated payment slang as a rejection: %+v", got)
	}
}

func TestActionableIntentOverridesBenignWrapper(t *testing.T) {
	cfg := appconfig.CommentModerationConfig{
		SemanticRules: appconfig.CommentModerationSemanticRulesConfig{
			Contexts: appconfig.CommentModerationSemanticContextConfig{
				UnambiguousBenignMarkers: []string{"违规样例"},
				ActionablePatterns:       []string{"确实可以找我"},
			},
		},
	}

	if isBenignSemanticClause(
		Normalize("这条低价账号广告只是违规样例。"),
		cfg,
	) != true {
		t.Fatal("pure violation sample should remain benign")
	}
	if isBenignSemanticClause(
		Normalize("下面是违规样例，低价账号确实可以找我。"),
		cfg,
	) {
		t.Fatal("actionable promotion must override benign wrapper")
	}
}

func TestSemanticContextComesFromConfiguration(t *testing.T) {
	cfg := appconfig.CommentModerationConfig{
		SemanticRules: appconfig.CommentModerationSemanticRulesConfig{
			Contexts: appconfig.CommentModerationSemanticContextConfig{
				ReportingMarkers: []string{"风险研究"},
			},
		},
	}
	signals := []Signal{
		{Source: SourceLexicon, Category: "spam_fraud", Level: LevelReview, Evidence: "低价出售"},
	}

	got := adjustSignalsBySemantics(Normalize("风险研究引用低价出售作为样例"), signals, cfg)
	if len(got) != 1 || got[0].Source != SourceSemantic {
		t.Fatalf("adjustSignalsBySemantics() = %+v, want configured suppression", got)
	}

	cfg.SemanticRules.Contexts.ReportingMarkers = nil
	got = adjustSignalsBySemantics(Normalize("风险研究引用低价出售作为样例"), signals, cfg)
	if len(got) != 1 || got[0].Source != SourceLexicon {
		t.Fatalf("adjustSignalsBySemantics() = %+v, want original signal", got)
	}
}

func TestSemanticTraceKeepsSuppressedSignals(t *testing.T) {
	cfg := appconfig.CommentModerationConfig{
		SemanticRules: appconfig.CommentModerationSemanticRulesConfig{
			Contexts: appconfig.CommentModerationSemanticContextConfig{
				ReportingMarkers: []string{"风险研究"},
			},
		},
	}
	signals := []Signal{
		{
			Source:   SourceLexicon,
			Category: "spam_fraud",
			Level:    LevelReview,
			Evidence: "低价出售",
			RuleID:   "lexicon",
			Clause:   1,
		},
	}

	adjusted, suppressed := adjustSignalsBySemanticsWithTrace(
		Normalize("风险研究引用低价出售作为样例"),
		signals,
		cfg,
	)
	if len(adjusted) != 1 || adjusted[0].Source != SourceSemantic {
		t.Fatalf("adjusted signals = %+v, want semantic allow", adjusted)
	}
	if len(suppressed) != 1 || suppressed[0].RuleID != "lexicon" || suppressed[0].Clause != 1 {
		t.Fatalf("suppressed signals = %+v, want original signal provenance", suppressed)
	}
}

func TestAbusePolicyOnlyKeepsSevereAttacks(t *testing.T) {
	cfg := appconfig.CommentModerationConfig{
		SemanticRules: appconfig.CommentModerationSemanticRulesConfig{
			AbusePolicy: appconfig.CommentModerationAbusePolicyConfig{
				SevereMarkers: []string{"脑子有坑"},
			},
		},
	}
	mild := []Signal{
		{Source: SourceLexicon, Category: "abuse", Level: LevelReview, Evidence: "胡说八道"},
	}
	got := adjustSignalsBySemantics(Normalize("这篇教程就是一本正经地胡说八道"), mild, cfg)
	if len(got) != 1 || got[0].Source != SourceSemantic {
		t.Fatalf("adjustSignalsBySemantics() mild = %+v, want semantic allow", got)
	}

	severe := []Signal{
		{Source: SourceLexicon, Category: "abuse", Level: LevelReview, Evidence: "脑子有坑"},
	}
	got = adjustSignalsBySemantics(Normalize("这作者脑子有坑"), severe, cfg)
	if len(got) != 1 || got[0].Source != SourceLexicon {
		t.Fatalf("adjustSignalsBySemantics() severe = %+v, want original risk", got)
	}
}

func TestValidateConfigRejectsInvalidPolicy(t *testing.T) {
	tests := []struct {
		name string
		cfg  appconfig.CommentModerationConfig
	}{
		{
			name: "duplicate rule id",
			cfg: appconfig.CommentModerationConfig{
				CombinationRules: []appconfig.CommentModerationCombinationRuleConfig{
					{ID: "duplicate", Subjects: []string{"账号"}, Predicates: []string{"出售"}},
					{ID: "duplicate", Subjects: []string{"会员"}, Predicates: []string{"低价"}},
				},
			},
		},
		{
			name: "invalid semantic pattern",
			cfg: appconfig.CommentModerationConfig{
				SemanticRules: appconfig.CommentModerationSemanticRulesConfig{
					Contexts: appconfig.CommentModerationSemanticContextConfig{
						ActionablePatterns: []string{"("},
					},
				},
			},
		},
		{
			name: "unsafe fuzzy distance",
			cfg: appconfig.CommentModerationConfig{
				Lexicon: appconfig.CommentModerationLexiconConfig{
					Fuzzy: appconfig.CommentModerationFuzzyConfig{
						MaxDistance: 3,
						CandidateWords: map[string][]string{
							"spam_fraud": {"论文代写"},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateConfig(tt.cfg); err == nil {
				t.Fatal("ValidateConfig() error = nil")
			}
		})
	}
}

func TestModeratorReviewsConfiguredCombinationRule(t *testing.T) {
	cfg := appconfig.CommentModerationConfig{
		CombinationRules: []appconfig.CommentModerationCombinationRuleConfig{
			{
				ID:         "implicit_game_boosting",
				Category:   "spam_fraud",
				Level:      LevelReview,
				Subjects:   []string{"段位"},
				Predicates: []string{"把号给我"},
			},
		},
		SemanticRules: testSemanticRules(),
	}
	moderator := NewModerator(staticModerationConfig{cfg: cfg}, zap.NewNop(), nil)
	result := moderator.ModerateWithBehavior(
		context.Background(),
		Request{Content: "段位卡住的把号给我，睡一觉起来就能看到变化。"},
		nil,
	)
	if result.Status == commentModel.StatusApproved {
		t.Fatal("ModerateWithBehavior() approved configured risk combination")
	}
	if len(result.Signals) == 0 || result.Signals[0].RuleID != "implicit_game_boosting" {
		t.Fatalf("ModerateWithBehavior() signals = %+v", result.Signals)
	}
}

func testSemanticRules() appconfig.CommentModerationSemanticRulesConfig {
	return appconfig.CommentModerationSemanticRulesConfig{
		Contexts: appconfig.CommentModerationSemanticContextConfig{
			RejectionMarkers:  []string{"不应该", "不需要"},
			ActionableMarkers: []string{"去签名"},
		},
	}
}

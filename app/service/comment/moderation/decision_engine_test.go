package moderation

import (
	"context"
	"math"
	"testing"

	"go.uber.org/zap"

	commentModel "meta-api/app/model/comment"
	appconfig "meta-api/config"
)

type staticContextAnalyzer struct {
	assessment ContextAssessment
	err        error
}

func (s staticContextAnalyzer) Analyze(context.Context, ContextInput,
	appconfig.CommentModerationConfig,
) (ContextAssessment, error) {
	return s.assessment, s.err
}

func TestEvidenceFromSignalsDeduplicatesCorrelatedDetectors(t *testing.T) {
	signals := []Signal{
		{Source: SourceLexicon, Category: "spam_fraud", Level: LevelReview, Evidence: "低价出售", Clause: 1},
		{Source: SourceSimilarity, Category: "spam_fraud", Level: LevelReview, Evidence: "低价出售", Clause: 1},
	}
	evidence := evidenceFromSignals(signals)
	if len(evidence) != 1 {
		t.Fatalf("evidenceFromSignals() = %+v, want one correlated evidence item", evidence)
	}
	if evidence[0].Source != SourceLexicon {
		t.Fatalf("kept source = %q, want strongest lexicon evidence", evidence[0].Source)
	}
}

func TestEvidenceBaselineUsesFusionInsteadOfAdditiveRuleScores(t *testing.T) {
	cfg := appconfig.CommentModerationConfig{}

	approved := baselineDecision(nil, cfg)
	if approved.Status != commentModel.StatusApproved || approved.Score != 0 ||
		approved.Decision != "evidence_baseline_allow" {
		t.Fatalf("empty evidence baseline = %+v", approved)
	}

	review := baselineDecision([]Signal{{
		Source: SourceLexicon, Category: "abuse", Level: LevelReview, Evidence: "样本一", Clause: 1,
	}}, cfg)
	if review.Status != commentModel.StatusPending || review.Score != 62 ||
		review.Decision != "evidence_baseline_review" {
		t.Fatalf("single evidence baseline = %+v", review)
	}

	rejected := baselineDecision([]Signal{
		{Source: SourceLexicon, Category: "abuse", Level: LevelReview, Evidence: "样本一", Clause: 1},
		{Source: SourceContext, Category: "abuse", Level: LevelReview, Evidence: "样本二", Clause: 1},
		{Source: SourceSemantic, Category: "abuse", Level: LevelReview, Evidence: "样本三", Clause: 1},
	}, cfg)
	if rejected.Status != commentModel.StatusRejected || rejected.Score < 90 ||
		rejected.Decision != "evidence_baseline_reject" {
		t.Fatalf("corroborated evidence baseline = %+v", rejected)
	}
}

func TestMergeEvidenceDeduplicatesLocalCandidateAndDetector(t *testing.T) {
	evidence, deduplicated := mergeEvidenceWithTrace(
		[]Evidence{{ID: "detector", Category: "abuse", Polarity: "positive", Confidence: 0.62,
			CorrelationGroup: "clause:1:abuse:傻逼"}},
		[]Evidence{{ID: "local", Category: "abuse", Polarity: "positive", Confidence: 0.92,
			CorrelationGroup: "clause:1:abuse:傻逼"}},
	)
	if len(evidence) != 1 || evidence[0].ID != "local" {
		t.Fatalf("mergeEvidence() = %+v, want strongest correlated item", evidence)
	}
	if len(deduplicated) != 1 || deduplicated[0].Discarded.ID != "detector" ||
		deduplicated[0].KeptID != "local" {
		t.Fatalf("deduplication trace = %+v", deduplicated)
	}
}

func TestFuseEvidenceStrengthensIndependentSignalsAndAppliesCounterEvidence(t *testing.T) {
	cfg := appconfig.CommentModerationDecisionEngineConfig{}
	one := fuseEvidence([]Evidence{
		{Category: "abuse", Polarity: "positive", Confidence: 0.6, CorrelationGroup: "g1"},
	}, ContextAssessment{}, cfg)
	three := fuseEvidence([]Evidence{
		{Category: "abuse", Polarity: "positive", Confidence: 0.6, CorrelationGroup: "g1"},
		{Category: "abuse", Polarity: "positive", Confidence: 0.6, CorrelationGroup: "g2"},
		{Category: "abuse", Polarity: "positive", Confidence: 0.6, CorrelationGroup: "g3"},
	}, ContextAssessment{}, cfg)
	if one.Status != commentModel.StatusPending {
		t.Fatalf("one weak signal status = %q, want pending", one.Status)
	}
	if three.Status != commentModel.StatusRejected || three.RiskProbability <= one.RiskProbability {
		t.Fatalf("three independent signals = %+v, want stronger rejected decision than %+v", three, one)
	}

	withCounterEvidence := fuseEvidence([]Evidence{
		{Category: "abuse", Polarity: "positive", Confidence: 0.6, CorrelationGroup: "g1"},
		{Category: "abuse", Polarity: "negative", Confidence: 0.8, CorrelationGroup: "counter"},
	}, ContextAssessment{}, cfg)
	if withCounterEvidence.RiskProbability >= one.RiskProbability {
		t.Fatalf("counter evidence risk = %.4f, want less than %.4f",
			withCounterEvidence.RiskProbability, one.RiskProbability)
	}
	_, fusion := fuseEvidenceWithTrace([]Evidence{
		{Category: "abuse", Polarity: "positive", Confidence: 0.6, CorrelationGroup: "g1"},
		{Category: "abuse", Polarity: "negative", Confidence: 0.8, CorrelationGroup: "counter"},
	}, ContextAssessment{}, cfg)
	if fusion.Thresholds.ApproveMax != 0.2 || fusion.Thresholds.RejectMin != 0.9 ||
		len(fusion.Categories) != 1 || fusion.Categories[0].CounterEvidence != 0.8 {
		t.Fatalf("fusion trace = %+v", fusion)
	}
}

func TestFuseEvidenceAppliesReportingContextToGenericStructureCategory(t *testing.T) {
	decision := fuseEvidence([]Evidence{
		{Category: "contact", Polarity: "positive", Confidence: 0.58, CorrelationGroup: "contact"},
	}, ContextAssessment{
		Analyzed:          true,
		Confidence:        0.92,
		Intent:            "reporting",
		BenignProbability: 0.94,
		CategoryProbabilities: map[string]float64{
			"contact": 0.58,
		},
	}, appconfig.CommentModerationDecisionEngineConfig{})
	if decision.Status != commentModel.StatusApproved || decision.RiskProbability > 0.2 {
		t.Fatalf("generic structure category decision = %+v, want reporting context to approve", decision)
	}
}

func TestDecisionEngineUsesLocalVariantCandidateDirectly(t *testing.T) {
	cfg := appconfig.CommentModerationConfig{
		DecisionEngine: appconfig.CommentModerationDecisionEngineConfig{
			ContextAnalysis: appconfig.CommentModerationContextAnalysisConfig{
				MaxCandidates: 16,
			},
		},
	}
	moderator := NewModerator(staticModerationConfig{cfg: cfg}, zap.NewNop(), nil)
	result := moderator.ModerateWithBehavior(context.Background(), Request{Content: "你真是个sb"}, nil)

	if result.Trace.DecisionEngine == nil {
		t.Fatal("missing decision engine trace")
	}
	trace := result.Trace.DecisionEngine
	if !trace.Context.Analyzed || !trace.Decision.Actionable {
		t.Fatalf("local direct decision = %+v, context = %+v", trace.Decision, trace.Context)
	}
	if !containsRewriteCandidate(trace.Candidates, "sb", "傻逼") {
		t.Fatalf("candidates = %+v, want sb -> 傻逼", trace.Candidates)
	}
	if result.Trace.Behavior.Status != "skipped" || !result.Trace.Decisions.Probability.Evaluated ||
		result.Trace.Decisions.Final.Status != result.Status {
		t.Fatalf("pipeline trace = %+v", result.Trace)
	}
}

func TestDecisionEngineRequiresContextConfidence(t *testing.T) {
	cfg := appconfig.CommentModerationConfig{
		DecisionEngine: appconfig.CommentModerationDecisionEngineConfig{
			Thresholds: appconfig.CommentModerationProbabilityThresholdConfig{
				MinConfidence: 0.8,
			},
		},
	}
	analyzer := staticContextAnalyzer{assessment: ContextAssessment{
		Analyzed:   true,
		Confidence: 0.79,
		CategoryProbabilities: map[string]float64{
			"abuse": 1,
		},
	}}
	moderator := NewModeratorWithContextAnalyzer(staticModerationConfig{cfg: cfg}, zap.NewNop(), nil, analyzer)
	result := moderator.ModerateWithBehavior(context.Background(), Request{Content: "普通缩写"}, nil)
	if result.Status != commentModel.StatusApproved {
		t.Fatalf("low-confidence decision status = %q, want safe fallback approved", result.Status)
	}
	if result.Trace.DecisionEngine.Decision.Actionable ||
		result.Trace.DecisionEngine.Decision.FallbackReason != "context_confidence_below_threshold" {
		t.Fatalf("low-confidence probability decision = %+v", result.Trace.DecisionEngine.Decision)
	}
	if math.IsNaN(result.Trace.DecisionEngine.Decision.RiskProbability) {
		t.Fatal("risk probability is NaN")
	}
	if result.Trace.Decisions.Probability.Applied ||
		result.Trace.Decisions.Probability.Reason != "context_confidence_below_threshold" {
		t.Fatalf("probability application trace = %+v", result.Trace.Decisions.Probability)
	}
}

func TestDecisionEngineCannotOverrideHardSafetyEvidence(t *testing.T) {
	trace := &DecisionEngineTrace{
		Evidence: []Evidence{
			{Source: SourceStructure, RuleID: "script_injection", Polarity: "positive"},
		},
		Decision: ProbabilityDecision{
			Status:          commentModel.StatusApproved,
			RiskProbability: 0.01,
			Actionable:      true,
		},
	}
	result := applyDecisionEngine(Result{Status: commentModel.StatusRejected, Score: 80}, trace)
	if result.Status != commentModel.StatusRejected || result.Score != 80 {
		t.Fatalf("hard safety result = %+v, want original rejection", result)
	}
	if trace.Decision.Actionable || trace.Decision.FallbackReason != "hard_safety_signal" {
		t.Fatalf("hard safety probability decision = %+v", trace.Decision)
	}
}

func TestDecisionFlowShowsHardSafetyBlockingProbabilityOverride(t *testing.T) {
	trace := &DecisionEngineTrace{
		Evidence: []Evidence{{Source: SourceStructure, RuleID: "script_injection", Polarity: "positive"}},
		Decision: ProbabilityDecision{
			Status: commentModel.StatusApproved, RiskProbability: 0.01, Actionable: true,
		},
	}
	flow := DecisionFlowTrace{}
	result := applyDecisionEngineWithTrace(
		Result{Status: commentModel.StatusRejected, Score: 100, Decision: "evidence_baseline_reject"}, trace, &flow,
	)
	if result.Status != commentModel.StatusRejected || !flow.HardSafety.Triggered ||
		flow.Probability.Applied || flow.Final.Status != commentModel.StatusRejected {
		t.Fatalf("hard safety decision flow = %+v, result = %+v", flow, result)
	}
}

func containsRewriteCandidate(candidates []RewriteCandidate, observed, canonical string) bool {
	for _, candidate := range candidates {
		if candidate.Observed == observed && candidate.Text == canonical {
			return true
		}
	}
	return false
}

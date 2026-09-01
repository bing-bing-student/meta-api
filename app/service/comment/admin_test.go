package comment

import (
	"testing"

	commentModeration "meta-api/app/service/comment/moderation"
)

func TestNormalizeAdminAuthorHandle(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "empty", value: " ", want: ""},
		{name: "short numeric", value: "1", want: "00001"},
		{name: "already padded", value: "00001", want: "00001"},
		{name: "five digits", value: "99999", want: "99999"},
		{name: "six digits", value: "100000", want: "100000"},
		{name: "non numeric", value: "github-user", want: "github-user"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeAdminAuthorHandle(tt.value); got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestAdminModerationPreviewKeepsPipelineTrace(t *testing.T) {
	clauseText := commentModeration.Normalize("风险研究引用低价出售作为样例")
	suppressed := commentModeration.Signal{
		Source:   commentModeration.SourceLexicon,
		Category: "spam_fraud",
		Level:    commentModeration.LevelReview,
		Evidence: "低价出售",
		RuleID:   "lexicon",
		Clause:   1,
	}
	result := commentModeration.Result{
		Status:   "approved",
		Decision: "evidence_baseline_allow",
		Signals: []commentModeration.Signal{
			{
				Source:   commentModeration.SourceSemantic,
				Category: "benign_context",
				Level:    commentModeration.LevelAllow,
				RuleID:   "benign_context",
			},
		},
		Trace: commentModeration.Trace{
			Clauses: []commentModeration.ClauseTrace{
				{ID: 1, Text: clauseText},
			},
			DetectorSignals:   []commentModeration.Signal{suppressed},
			SuppressedSignals: []commentModeration.Signal{suppressed},
			Behavior: commentModeration.BehaviorTrace{
				Status: "executed", ReadOnly: true, ContextProvided: true,
				Metrics: []commentModeration.BehaviorMetricTrace{{
					Name: "user_frequency", Evaluated: true, ObservedCount: 2,
					ProspectiveCount: 3, ReviewThreshold: 6, WindowSeconds: 600,
				}},
			},
			Decisions: commentModeration.DecisionFlowTrace{
				Rule:  commentModeration.DecisionSnapshot{Status: "pending", Score: 62, Decision: "evidence_baseline_review"},
				Final: commentModeration.DecisionSnapshot{Status: "approved", Score: 12, Decision: "probability_allow"},
			},
			DecisionEngine: &commentModeration.DecisionEngineTrace{
				Candidates: []commentModeration.RewriteCandidate{
					{Text: "低价出售", Observed: "低价岀售", Category: "spam_fraud",
						Role:   commentModeration.CandidateRolePredicate,
						Method: "pinyin_homophone", Confidence: 0.8, Clause: 1},
				},
				Evidence: []commentModeration.Evidence{
					{ID: "e001", Source: "lexicon", Category: "spam_fraud", Confidence: 0.62},
				},
				Context: commentModeration.ContextAssessment{
					Analyzed:    true,
					Confidence:  0.86,
					Intent:      "reporting",
					Explanation: "命中举报、引用、研究或案例说明语境",
				},
				Fusion: commentModeration.EvidenceFusionTrace{
					Thresholds: commentModeration.ProbabilityThresholdTrace{
						ApproveMax: 0.2, RejectMin: 0.9, MinConfidence: 0.7,
					},
					Categories: []commentModeration.CategoryFusionTrace{{
						Category: "spam_fraud", RuleRisk: 0.62, FinalRisk: 0.12,
					}},
				},
				Decision: commentModeration.ProbabilityDecision{
					Status:          "approved",
					RiskProbability: 0.12,
					Calibration:     "bootstrap-uncalibrated",
				},
			},
		},
	}

	item := toAdminPreviewCommentModerationItem(7, clauseText.Raw, result)
	if len(item.Trace.Clauses) != 1 || item.Trace.Clauses[0].ID != 1 {
		t.Fatalf("trace clauses = %+v, want clause 1", item.Trace.Clauses)
	}
	if len(item.Trace.SuppressedSignals) != 1 || item.Trace.SuppressedSignals[0].ClauseID != 1 {
		t.Fatalf("suppressed signals = %+v, want clause provenance", item.Trace.SuppressedSignals)
	}
	if item.Trace.Behavior.Status != "executed" || !item.Trace.Behavior.ReadOnly {
		t.Fatal("behavior evaluation state was not preserved")
	}
	if len(item.Trace.Behavior.Metrics) != 1 || item.Trace.Behavior.Metrics[0].ProspectiveCount != 3 ||
		item.Trace.Decisions.Final.Status != "approved" {
		t.Fatalf("execution and decision flow trace = %+v", item.Trace)
	}
	if item.Text.Confusable != clauseText.Confusable {
		t.Fatalf("confusable = %q, want %q", item.Text.Confusable, clauseText.Confusable)
	}
	if item.Trace.DecisionEngine == nil || item.Trace.DecisionEngine.Decision.RiskProbability != 0.12 {
		t.Fatalf("decision engine trace = %+v, want mapped probability decision", item.Trace.DecisionEngine)
	}
	if len(item.Trace.DecisionEngine.Candidates) != 1 || len(item.Trace.DecisionEngine.Evidence) != 1 {
		t.Fatalf("decision engine debug data = %+v", item.Trace.DecisionEngine)
	}
	if item.Trace.DecisionEngine.Fusion.Thresholds.RejectMin != 0.9 ||
		len(item.Trace.DecisionEngine.Fusion.Categories) != 1 {
		t.Fatalf("fusion trace = %+v", item.Trace.DecisionEngine.Fusion)
	}
	if !item.Trace.DecisionEngine.Context.Analyzed || item.Trace.DecisionEngine.Context.Intent != "reporting" {
		t.Fatalf("local context trace = %+v", item.Trace.DecisionEngine.Context)
	}
	if candidate := item.Trace.DecisionEngine.Candidates[0]; candidate.Observed != "低价岀售" ||
		candidate.Category != "spam_fraud" || candidate.Role != commentModeration.CandidateRolePredicate ||
		candidate.ClauseID != 1 {
		t.Fatalf("mapped rewrite candidate = %+v", candidate)
	}
}

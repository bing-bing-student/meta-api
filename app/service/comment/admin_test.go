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
		Decision: "no_risk_signal",
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
			BehaviorEvaluated: true,
		},
	}

	item := toAdminPreviewCommentModerationItem(7, clauseText.Raw, result)
	if len(item.Trace.Clauses) != 1 || item.Trace.Clauses[0].ID != 1 {
		t.Fatalf("trace clauses = %+v, want clause 1", item.Trace.Clauses)
	}
	if len(item.Trace.SuppressedSignals) != 1 || item.Trace.SuppressedSignals[0].ClauseID != 1 {
		t.Fatalf("suppressed signals = %+v, want clause provenance", item.Trace.SuppressedSignals)
	}
	if !item.Trace.BehaviorEvaluated {
		t.Fatal("behavior evaluation state was not preserved")
	}
	if item.Text.Confusable != clauseText.Confusable {
		t.Fatalf("confusable = %q, want %q", item.Text.Confusable, clauseText.Confusable)
	}
}

package comment

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	commentModel "meta-api/app/model/comment"
	"meta-api/app/service/comment/moderation"
	"meta-api/common/types"
	appconfig "meta-api/config"
)

func moderationFeedbackTestConfig() *appconfig.Config {
	return &appconfig.Config{CommentModerationConfig: &appconfig.CommentModerationConfig{
		Categories: map[string]appconfig.CommentModerationCategoryConfig{
			"abuse": {FeedbackEnabled: true}, "spam_fraud": {FeedbackEnabled: true},
		},
	}}
}

type adminReviewModelStub struct {
	commentModel.Model
	comment  *commentModel.Comment
	audit    *commentModel.CommentModerationAudit
	row      *commentModel.AdminListItem
	reviewed *commentModel.CommentModerationFeedback
	status   string
}

func (s *adminReviewModelStub) GetCommentByID(context.Context, uint64) (*commentModel.Comment, error) {
	return s.comment, nil
}

func (s *adminReviewModelStub) GetModerationAuditByID(context.Context,
	uint64,
) (*commentModel.CommentModerationAudit, error) {
	return s.audit, nil
}

func (s *adminReviewModelStub) ReviewComment(_ context.Context, _ uint64, status string, _ time.Time,
	feedback *commentModel.CommentModerationFeedback,
) error {
	s.status = status
	s.reviewed = feedback
	return nil
}

func (s *adminReviewModelStub) GetAdminCommentByID(context.Context, uint64) (*commentModel.AdminListItem, error) {
	return s.row, nil
}

func (s *adminReviewModelStub) GetLatestModerationAuditByCommentID(context.Context,
	uint64,
) (*commentModel.CommentModerationAudit, error) {
	return s.audit, nil
}

type moderationFeedbackModelStub struct {
	commentModel.Model
	policy *commentModel.ModerationFeedbackPolicy
	err    error
}

func (s moderationFeedbackModelStub) ResolveModerationFeedbackPolicy(context.Context, string,
	string,
) (*commentModel.ModerationFeedbackPolicy, error) {
	return s.policy, s.err
}

func (s moderationFeedbackModelStub) GetModerationAuditByID(context.Context,
	uint64,
) (*commentModel.CommentModerationAudit, error) {
	return &commentModel.CommentModerationAudit{ID: 1}, nil
}

func (s moderationFeedbackModelStub) CreateModerationFeedback(context.Context,
	*commentModel.CommentModerationFeedback,
) error {
	return nil
}

func TestAdminModerationFeedbackCategoryValidation(t *testing.T) {
	service := &commentService{config: moderationFeedbackTestConfig(), commentModel: moderationFeedbackModelStub{}}
	tests := []struct {
		name     string
		status   string
		category string
		valid    bool
	}{
		{name: "approved without category", status: commentModel.StatusApproved, valid: true},
		{name: "approved with category", status: commentModel.StatusApproved, category: "abuse"},
		{name: "pending requires category", status: commentModel.StatusPending},
		{name: "rejected rejects arbitrary category", status: commentModel.StatusRejected, category: "anything"},
		{name: "rejected accepts taxonomy category", status: commentModel.StatusRejected, category: "spam_fraud", valid: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := service.AdminSubmitCommentModerationFeedback(context.Background(),
				&types.AdminSubmitCommentModerationFeedbackRequest{
					AdminID: "1", AuditID: "1", ExpectedStatus: tt.status, ExpectedCategory: tt.category,
				})
			if tt.valid && err != nil {
				t.Fatalf("expected valid feedback, got %v", err)
			}
			if !tt.valid && !errors.Is(err, ErrInvalidComment) {
				t.Fatalf("expected ErrInvalidComment, got %v", err)
			}
		})
	}
}

func TestNewCommentModerationAuditKeepsSourceAndStructuralFingerprint(t *testing.T) {
	cfg := &appconfig.Config{CommentModerationConfig: &appconfig.CommentModerationConfig{}}
	service := &commentService{config: cfg}
	now := time.Date(2026, time.August, 29, 12, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	input := commentModerationInput{
		CommentID: 11, UserID: 22, ArticleID: 33, Content: "  我这里有资料  ", Now: now,
	}
	result := commentModerationResult{
		Status: commentModel.StatusPending,
		Score:  84,
		Trace: moderation.Trace{DecisionEngine: &moderation.DecisionEngineTrace{
			Context: moderation.ContextAssessment{Relations: []moderation.SemanticRelation{{
				ID: "r001", Clause: 1, Subject: "我", Action: "提供", Object: "资料",
				Category: "spam_fraud", Inferred: true, Confidence: 0.86,
			}}},
			Decision: moderation.ProbabilityDecision{RiskProbability: 0.8436},
		}},
		Decision: "probability_review",
	}

	audit, err := service.newCommentModerationAudit(commentModel.ModerationAuditSourceAdminSimulation,
		"batch-1", 44, input, result)
	if err != nil {
		t.Fatalf("newCommentModerationAudit() error = %v", err)
	}
	if audit.Source != commentModel.ModerationAuditSourceAdminSimulation || audit.BatchID != "batch-1" ||
		audit.OperatorID != 44 || audit.CommentID != 11 || audit.UserID != 22 || audit.ArticleID != 33 {
		t.Fatalf("audit identity = %+v", audit)
	}
	if audit.ContentHash != moderationContentHash("我这里有资料") || audit.RelationFingerprint == "" {
		t.Fatalf("audit hashes = content %q, relation %q", audit.ContentHash, audit.RelationFingerprint)
	}
	if !strings.HasPrefix(audit.PolicyVersion, "local-") || audit.RiskProbability != 0.8436 ||
		audit.Status != commentModel.StatusPending || audit.RiskScore != 84 || audit.Decision != "probability_review" {
		t.Fatalf("audit decision snapshot = %+v", audit)
	}
	if !strings.Contains(audit.RequestSnapshot, "我这里有资料") ||
		!strings.Contains(audit.ResultSnapshot, "probability_review") || !audit.CreateTime.Equal(now) {
		t.Fatalf("audit JSON snapshot = %+v", audit)
	}
}

func TestDecodeCommentModerationAuditResultRequiresCurrentSnapshot(t *testing.T) {
	result := commentModerationResult{Status: commentModel.StatusPending, Score: 62, Decision: "probability_review"}
	text := moderation.Normalize("ＶＸ：рromo_code，詳情私聊。")
	envelope, err := json.Marshal(commentModerationAuditResultSnapshot{Result: result, Text: text})
	if err != nil {
		t.Fatal(err)
	}
	gotResult, gotText, err := decodeCommentModerationAuditResult(&commentModel.CommentModerationAudit{
		Content: text.Raw, ResultSnapshot: string(envelope),
	})
	if err != nil || gotResult.Decision != result.Decision || gotText.Confusable != text.Confusable {
		t.Fatalf("envelope decode = result %+v text %+v err %v", gotResult, gotText, err)
	}
	incomplete, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = decodeCommentModerationAuditResult(&commentModel.CommentModerationAudit{
		Content: text.Raw, ResultSnapshot: string(incomplete),
	})
	if err == nil {
		t.Fatal("snapshot without the current result/text envelope must be rejected")
	}
}

func TestAdminReviewCommentCanAtomicallyIncludeCalibrationFeedback(t *testing.T) {
	model := &adminReviewModelStub{
		comment: &commentModel.Comment{ID: 10, ArticleID: 30},
		audit: &commentModel.CommentModerationAudit{
			ID: 20, CommentID: 10, Source: commentModel.ModerationAuditSourceLiveComment,
		},
	}
	service := &commentService{config: moderationFeedbackTestConfig(), commentModel: model}
	err := service.AdminReviewComment(context.Background(), &types.AdminReviewCommentRequest{
		AdminID: "2", ID: "10", Status: commentModel.StatusRejected, IncludeFeedback: true,
		AuditID: "20", ExpectedCategory: "abuse", Note: "主体直接辱骂作者",
	})
	if err != nil {
		t.Fatalf("AdminReviewComment() error = %v", err)
	}
	if model.status != commentModel.StatusRejected || model.reviewed == nil ||
		model.reviewed.AuditID != 20 || model.reviewed.OperatorID != 2 ||
		model.reviewed.ExpectedCategory != "abuse" {
		t.Fatalf("review transaction = status %q feedback %+v", model.status, model.reviewed)
	}
}

func TestAdminGetCommentDetailKeepsCurrentStatusSeparateFromOriginalAudit(t *testing.T) {
	serviceForAudit := &commentService{}
	input := commentModerationInput{
		CommentID: 10, UserID: 2, ArticleID: 3, Content: "有资源的私聊我",
		ArticleTitle: "测试文章", ClientIP: "127.0.0.1", Now: time.Now(),
	}
	original := commentModerationResult{
		Status: commentModel.StatusPending, Score: 72, Decision: "probability_review",
		Trace: moderation.Trace{Decisions: moderation.DecisionFlowTrace{
			Final: moderation.DecisionSnapshot{Status: commentModel.StatusPending, Score: 72, Decision: "probability_review"},
		}},
	}
	audit, err := serviceForAudit.newCommentModerationAudit(commentModel.ModerationAuditSourceLiveComment,
		"", 0, input, original)
	if err != nil {
		t.Fatal(err)
	}
	audit.ID = 20
	model := &adminReviewModelStub{
		row: &commentModel.AdminListItem{
			ID: 10, Content: input.Content, Status: commentModel.StatusApproved,
			CreateTime: input.Now, UpdateTime: input.Now,
		},
		audit: &audit,
	}
	service := &commentService{config: moderationFeedbackTestConfig(), commentModel: model}
	detail, err := service.AdminGetCommentDetail(context.Background(), &types.AdminGetCommentDetailRequest{ID: "10"})
	if err != nil {
		t.Fatalf("AdminGetCommentDetail() error = %v", err)
	}
	if detail.Comment.Status != commentModel.StatusApproved || detail.Moderation == nil ||
		detail.Moderation.Result.Status != commentModel.StatusPending || detail.Moderation.AuditID != "20" {
		t.Fatalf("comment detail did not preserve current/original statuses: %+v", detail)
	}
	if len(detail.FeedbackCategories) == 0 || detail.Moderation.Context.UserID != "2" ||
		!detail.Moderation.Context.ClientIPProvided {
		t.Fatalf("comment detail context/taxonomy missing: %+v", detail)
	}
}

func TestAdminReviewCommentRejectsFeedbackForAnotherComment(t *testing.T) {
	model := &adminReviewModelStub{
		comment: &commentModel.Comment{ID: 10, ArticleID: 30},
		audit: &commentModel.CommentModerationAudit{
			ID: 20, CommentID: 99, Source: commentModel.ModerationAuditSourceLiveComment,
		},
	}
	service := &commentService{config: moderationFeedbackTestConfig(), commentModel: model}
	err := service.AdminReviewComment(context.Background(), &types.AdminReviewCommentRequest{
		AdminID: "2", ID: "10", Status: commentModel.StatusRejected, IncludeFeedback: true,
		AuditID: "20", ExpectedCategory: "abuse",
	})
	if !errors.Is(err, ErrInvalidComment) || model.reviewed != nil {
		t.Fatalf("expected invalid cross-comment feedback, got %v and %+v", err, model.reviewed)
	}
}

func TestModerationHasHardSafetyEvidence(t *testing.T) {
	result := commentModerationResult{Trace: moderation.Trace{DecisionEngine: &moderation.DecisionEngineTrace{
		Evidence: []moderation.Evidence{{
			Source: moderation.SourceStructure, RuleID: "script_injection", Polarity: "positive",
		}},
	}}}
	if !moderationHasHardSafetyEvidence(result) {
		t.Fatal("script injection must not be downgraded by human feedback")
	}
}

func TestConfirmedSimulationFeedbackCanChangeRealCommentDecision(t *testing.T) {
	service := &commentService{commentModel: moderationFeedbackModelStub{policy: &commentModel.ModerationFeedbackPolicy{
		ExpectedStatus: commentModel.StatusRejected,
		Support:        1,
		ExactContent:   true,
	}}}
	result := service.applyModerationFeedbackPolicy(context.Background(),
		commentModerationInput{Content: "管理员模拟中已确认的样本"},
		commentModerationResult{Status: commentModel.StatusApproved})
	if result.Status != commentModel.StatusRejected || result.Score != 100 ||
		result.Decision != "human_feedback_exact" {
		t.Fatalf("feedback-adjusted result = %+v", result)
	}
	feedback := result.Trace.Decisions.Feedback
	if !feedback.Evaluated || !feedback.Matched || !feedback.Consensus || !feedback.Applied ||
		feedback.Scope != "exact" || feedback.After.Status != commentModel.StatusRejected {
		t.Fatalf("feedback decision trace = %+v", feedback)
	}
}

func TestConfirmedFeedbackCannotDowngradeHardSafetyDecision(t *testing.T) {
	service := &commentService{commentModel: moderationFeedbackModelStub{policy: &commentModel.ModerationFeedbackPolicy{
		ExpectedStatus: commentModel.StatusApproved,
		Support:        1,
		ExactContent:   true,
	}}}
	original := commentModerationResult{
		Status: commentModel.StatusRejected,
		Score:  100,
		Trace: moderation.Trace{DecisionEngine: &moderation.DecisionEngineTrace{Evidence: []moderation.Evidence{{
			Source: moderation.SourceStructure, RuleID: "script_injection", Polarity: "positive",
		}}}},
	}
	result := service.applyModerationFeedbackPolicy(context.Background(),
		commentModerationInput{Content: "<script>alert(1)</script>"}, original)
	if result.Status != original.Status || result.Score != original.Score {
		t.Fatalf("hard safety result was downgraded: %+v", result)
	}
	if result.Trace.Decisions.Feedback.Applied ||
		result.Trace.Decisions.Feedback.Reason != "hard_safety_signal" {
		t.Fatalf("hard safety feedback trace = %+v", result.Trace.Decisions.Feedback)
	}
}

func TestUnconfirmedFeedbackConsensusIsVisibleButNotApplied(t *testing.T) {
	service := &commentService{commentModel: moderationFeedbackModelStub{policy: &commentModel.ModerationFeedbackPolicy{
		ExpectedStatus: commentModel.StatusRejected,
		Support:        1, Total: 2, Conflicts: 1, RequiredSupport: 1,
		Consensus: false, Applicable: false, ExactContent: true,
	}}}
	result := service.applyModerationFeedbackPolicy(context.Background(),
		commentModerationInput{Content: "反馈冲突样本"},
		commentModerationResult{Status: commentModel.StatusApproved, Decision: "probability_allow"})
	if result.Status != commentModel.StatusApproved || result.Trace.Decisions.Feedback.Applied ||
		result.Trace.Decisions.Feedback.Reason != "feedback_no_consensus" {
		t.Fatalf("conflicting feedback result = %+v", result)
	}
}

package comment

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	commentModel "meta-api/app/model/comment"
	commentModeration "meta-api/app/service/comment/moderation"
)

type commentModerationAuditResultSnapshot struct {
	Result commentModerationResult   `json:"result"`
	Text   commentModerationTextView `json:"text"`
}

func (s *commentService) newCommentModerationAudit(source, batchID string, operatorID uint64,
	input commentModerationInput, result commentModerationResult,
) (commentModel.CommentModerationAudit, error) {
	requestJSON, err := json.Marshal(input)
	if err != nil {
		return commentModel.CommentModerationAudit{}, fmt.Errorf("encode moderation audit request: %w", err)
	}
	resultJSON, err := json.Marshal(commentModerationAuditResultSnapshot{
		Result: result,
		Text:   commentModeration.Normalize(input.Content),
	})
	if err != nil {
		return commentModel.CommentModerationAudit{}, fmt.Errorf("encode moderation audit result: %w", err)
	}
	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}
	probability := 0.0
	relations := []commentModeration.SemanticRelation(nil)
	if trace := result.Trace.DecisionEngine; trace != nil {
		probability = trace.Decision.RiskProbability
		relations = trace.Context.Relations
	}
	return commentModel.CommentModerationAudit{
		Source:              source,
		BatchID:             strings.TrimSpace(batchID),
		CommentID:           input.CommentID,
		UserID:              input.UserID,
		ArticleID:           input.ArticleID,
		OperatorID:          operatorID,
		Content:             input.Content,
		ContentHash:         moderationContentHash(input.Content),
		RelationFingerprint: commentModeration.RelationFingerprint(relations),
		PolicyVersion:       s.commentModerationPolicyVersion(),
		RequestSnapshot:     string(requestJSON),
		ResultSnapshot:      string(resultJSON),
		Status:              result.Status,
		RiskScore:           result.Score,
		RiskProbability:     probability,
		Decision:            result.Decision,
		CreateTime:          now,
	}, nil
}

func (s *commentService) commentModerationPolicyVersion() string {
	if s == nil || s.config == nil {
		return "local-unknown"
	}
	encoded, err := json.Marshal(s.config.CommentModerationSnapshot())
	if err != nil {
		return "local-unknown"
	}
	digest := sha256.Sum256(encoded)
	return fmt.Sprintf("local-%x", digest[:8])
}

func moderationContentHash(content string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(content)))
	return fmt.Sprintf("%x", digest[:])
}

func (s *commentService) nextModerationBatchID(now time.Time) string {
	if s != nil && s.idGenerator != nil {
		if value, err := s.idGenerator.NextID(); err == nil {
			return strconv.FormatUint(value, 10)
		}
	}
	return fmt.Sprintf("simulation-%d", now.UnixNano())
}

func (s *commentService) applyModerationFeedbackPolicy(ctx context.Context, input commentModerationInput,
	result commentModerationResult,
) commentModerationResult {
	before := moderationDecisionSnapshot(result)
	feedbackTrace := commentModeration.FeedbackApplicationTrace{
		Before: before,
		After:  before,
	}
	if s == nil || s.commentModel == nil {
		feedbackTrace.Reason = "feedback_store_unavailable"
		return finalizeModerationFeedbackTrace(result, feedbackTrace)
	}
	feedbackTrace.Evaluated = true
	relations := []commentModeration.SemanticRelation(nil)
	if result.Trace.DecisionEngine != nil {
		relations = result.Trace.DecisionEngine.Context.Relations
	}
	policy, err := s.commentModel.ResolveModerationFeedbackPolicy(ctx, moderationContentHash(input.Content),
		commentModeration.RelationFingerprint(relations))
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("resolve moderation feedback policy failed", zap.Error(err))
		}
		feedbackTrace.Reason = "feedback_lookup_failed"
		return finalizeModerationFeedbackTrace(result, feedbackTrace)
	}
	if policy == nil {
		feedbackTrace.Reason = "no_matching_feedback"
		return finalizeModerationFeedbackTrace(result, feedbackTrace)
	}
	feedbackTrace.Matched = true
	feedbackTrace.Consensus = policy.Consensus
	feedbackTrace.Support = policy.Support
	feedbackTrace.Total = policy.Total
	feedbackTrace.Conflicts = policy.Conflicts
	feedbackTrace.SimulationSupport = policy.SimulationSupport
	feedbackTrace.LiveSupport = policy.LiveSupport
	feedbackTrace.ExpectedStatus = policy.ExpectedStatus
	feedbackTrace.ExpectedCategory = policy.ExpectedCategory
	feedbackTrace.Scope = "relation"
	if policy.ExactContent {
		feedbackTrace.Scope = "exact"
	}
	applicable := policy.Applicable
	if policy.RequiredSupport == 0 {
		// Compatibility for in-process callers and tests that construct a policy directly.
		applicable = true
		feedbackTrace.Consensus = true
		if feedbackTrace.Total == 0 {
			feedbackTrace.Total = feedbackTrace.Support
		}
	}
	if !applicable {
		feedbackTrace.Reason = "feedback_no_consensus"
		if policy.Consensus && policy.Support < policy.RequiredSupport {
			feedbackTrace.Reason = "feedback_support_below_minimum"
		}
		return finalizeModerationFeedbackTrace(result, feedbackTrace)
	}
	if !commentModel.IsValidStatus(policy.ExpectedStatus) {
		feedbackTrace.Reason = "feedback_status_invalid"
		return finalizeModerationFeedbackTrace(result, feedbackTrace)
	}
	if policy.ExpectedStatus != commentModel.StatusRejected && moderationHasHardSafetyEvidence(result) {
		feedbackTrace.Reason = "hard_safety_signal"
		return finalizeModerationFeedbackTrace(result, feedbackTrace)
	}
	result.Status = policy.ExpectedStatus
	switch policy.ExpectedStatus {
	case commentModel.StatusApproved:
		result.Score = 0
	case commentModel.StatusRejected:
		result.Score = 100
	default:
		result.Score = 50
	}
	scope := "relation"
	if policy.ExactContent {
		scope = "exact"
	}
	result.Decision = "human_feedback_" + scope
	result.Reasons = append(result.Reasons, fmt.Sprintf("human_feedback:%s:support=%d", scope, policy.Support))
	if policy.ExpectedCategory != "" {
		result.Signals = append(result.Signals, commentModeration.Signal{
			Source:   commentModeration.SourceSemantic,
			Category: policy.ExpectedCategory,
			Level:    commentModeration.LevelReview,
			Reason:   "human_feedback_policy",
			Evidence: scope,
			RuleID:   "human_feedback_policy",
		})
	}
	feedbackTrace.Applied = true
	feedbackTrace.After = moderationDecisionSnapshot(result)
	return finalizeModerationFeedbackTrace(result, feedbackTrace)
}

func moderationDecisionSnapshot(result commentModerationResult) commentModeration.DecisionSnapshot {
	return commentModeration.DecisionSnapshot{
		Status: result.Status, Score: result.Score, Decision: result.Decision,
	}
}

func finalizeModerationFeedbackTrace(result commentModerationResult,
	feedback commentModeration.FeedbackApplicationTrace,
) commentModerationResult {
	result.Trace.Decisions.Feedback = feedback
	result.Trace.Decisions.Final = moderationDecisionSnapshot(result)
	return result
}

func moderationHasHardSafetyEvidence(result commentModerationResult) bool {
	if result.Trace.DecisionEngine == nil {
		return false
	}
	for _, item := range result.Trace.DecisionEngine.Evidence {
		if item.Source == commentModeration.SourceStructure && item.RuleID == "script_injection" &&
			item.Polarity == "positive" {
			return true
		}
	}
	return false
}

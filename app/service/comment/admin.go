package comment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	commentModel "meta-api/app/model/comment"
	commentModeration "meta-api/app/service/comment/moderation"
	"meta-api/common/constants"
	"meta-api/common/idutil"
	"meta-api/common/types"
)

func (s *commentService) AdminGetCommentList(ctx context.Context,
	request *types.AdminGetCommentListRequest) (*types.AdminGetCommentListResponse, error) {

	status := strings.TrimSpace(request.Status)
	if status != "" && !commentModel.IsValidStatus(status) {
		return nil, ErrInvalidComment
	}

	filter := commentModel.AdminListFilter{
		ArticleTitle:   strings.TrimSpace(request.ArticleTitle),
		ContentKeyword: strings.TrimSpace(request.ContentKeyword),
		AuthorHandle:   normalizeAdminAuthorHandle(request.AuthorHandle),
		Status:         status,
		Offset:         (request.Page - 1) * request.PageSize,
		Limit:          request.PageSize,
	}

	if request.ArticleID != "" {
		articleID, err := idutil.ParseID("articleID", request.ArticleID)
		if err != nil {
			s.logger.Error("invalid article id", zap.Error(err))
			return nil, ErrInvalidComment
		}
		filter.ArticleID = articleID
	}

	startTime, endTime, err := parseAdminCommentTimeRange(request.CreateStartTime, request.CreateEndTime)
	if err != nil {
		s.logger.Error("invalid comment create time range", zap.Error(err))
		return nil, ErrInvalidComment
	}
	filter.CreateStartTime = startTime
	filter.CreateEndTime = endTime

	rows, total, err := s.commentModel.ListComments(ctx, filter)
	if err != nil {
		s.logger.Error("failed to list comments", zap.Error(err))
		return nil, err
	}

	responseRows := make([]types.AdminCommentItem, 0, len(rows))
	for _, row := range rows {
		responseRows = append(responseRows, toAdminCommentItem(row))
	}

	return &types.AdminGetCommentListResponse{
		Rows:  responseRows,
		Total: int(total),
	}, nil
}

func (s *commentService) AdminGetCommentDetail(ctx context.Context,
	request *types.AdminGetCommentDetailRequest,
) (*types.AdminGetCommentDetailResponse, error) {
	if request == nil || s.commentModel == nil {
		return nil, ErrInvalidComment
	}
	id, err := idutil.ParseID("commentID", request.ID)
	if err != nil {
		return nil, ErrInvalidComment
	}
	row, err := s.commentModel.GetAdminCommentByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCommentNotFound
		}
		return nil, err
	}
	response := &types.AdminGetCommentDetailResponse{
		Comment:            toAdminCommentItem(*row),
		FeedbackCategories: commentModeration.FeedbackCategories(s.config.CommentModerationSnapshot()),
	}
	audit, err := s.commentModel.GetLatestModerationAuditByCommentID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return response, nil
		}
		return nil, err
	}
	result, text, err := decodeCommentModerationAuditResult(audit)
	if err != nil {
		return nil, err
	}
	var input commentModerationInput
	if err = json.Unmarshal([]byte(audit.RequestSnapshot), &input); err != nil {
		return nil, fmt.Errorf("decode moderation audit request: %w", err)
	}
	item := toAdminPreviewCommentModerationItem(0, audit.Content, result)
	item.AuditID = strconv.FormatUint(audit.ID, 10)
	item.Text = toAdminCommentModerationTextView(text)
	response.Moderation = &types.AdminCommentModerationAuditDetail{
		AuditID:       strconv.FormatUint(audit.ID, 10),
		Source:        audit.Source,
		PolicyVersion: audit.PolicyVersion,
		EvaluatedAt:   audit.CreateTime.Format(time.RFC3339Nano),
		Context: types.AdminCommentModerationExecutionContext{
			UserID:                  optionalUint64String(input.UserID),
			ArticleID:               optionalUint64String(input.ArticleID),
			ClientIPProvided:        strings.TrimSpace(input.ClientIP) != "",
			ArticleTitleProvided:    strings.TrimSpace(input.ArticleTitle) != "",
			ArticleCategoryProvided: strings.TrimSpace(input.ArticleCategory) != "",
			ParentContentProvided:   strings.TrimSpace(input.ParentContent) != "",
			ReplyContentProvided:    strings.TrimSpace(input.ReplyToContent) != "",
		},
		Result: item,
	}
	return response, nil
}

func decodeCommentModerationAuditResult(audit *commentModel.CommentModerationAudit,
) (commentModerationResult, commentModerationTextView, error) {
	if audit == nil {
		return commentModerationResult{}, commentModerationTextView{}, ErrInvalidComment
	}
	var snapshot commentModerationAuditResultSnapshot
	if err := json.Unmarshal([]byte(audit.ResultSnapshot), &snapshot); err != nil {
		return commentModerationResult{}, commentModerationTextView{},
			fmt.Errorf("decode moderation audit result: %w", err)
	}
	if snapshot.Result.Status == "" || snapshot.Text.Raw == "" {
		return commentModerationResult{}, commentModerationTextView{},
			errors.New("moderation audit does not use the current result snapshot schema")
	}
	return snapshot.Result, snapshot.Text, nil
}

func optionalUint64String(value uint64) string {
	if value == 0 {
		return ""
	}
	return strconv.FormatUint(value, 10)
}

func (s *commentService) AdminUpdateCommentStatus(ctx context.Context,
	request *types.AdminUpdateCommentStatusRequest) error {

	status := strings.TrimSpace(request.Status)
	if !commentModel.IsValidStatus(status) {
		return ErrInvalidComment
	}

	id, err := idutil.ParseID("commentID", request.ID)
	if err != nil {
		s.logger.Error("invalid comment id", zap.Error(err))
		return ErrInvalidComment
	}

	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		s.logger.Error("failed to load location", zap.Error(err))
		return fmt.Errorf("failed to load location: %w", err)
	}
	if err = s.commentModel.UpdateCommentStatus(ctx, id, status, time.Now().In(loc)); err != nil {
		s.logger.Error("failed to update comment status", zap.Error(err))
		return err
	}

	return nil
}

func (s *commentService) AdminReviewComment(ctx context.Context,
	request *types.AdminReviewCommentRequest,
) error {
	if request == nil || s.commentModel == nil {
		return ErrInvalidComment
	}
	status := strings.TrimSpace(request.Status)
	if !commentModel.IsValidStatus(status) {
		return ErrInvalidComment
	}
	commentID, err := idutil.ParseID("commentID", request.ID)
	if err != nil {
		return ErrInvalidComment
	}
	if _, err = s.commentModel.GetCommentByID(ctx, commentID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrCommentNotFound
		}
		return err
	}
	var feedback *commentModel.CommentModerationFeedback
	if request.IncludeFeedback {
		auditID, parseErr := idutil.ParseID("auditID", request.AuditID)
		if parseErr != nil {
			return ErrInvalidComment
		}
		operatorID, parseErr := idutil.ParseID("adminID", request.AdminID)
		if parseErr != nil {
			return ErrInvalidComment
		}
		audit, auditErr := s.commentModel.GetModerationAuditByID(ctx, auditID)
		if auditErr != nil {
			if errors.Is(auditErr, gorm.ErrRecordNotFound) {
				return ErrCommentNotFound
			}
			return auditErr
		}
		if audit.CommentID != commentID || audit.Source != commentModel.ModerationAuditSourceLiveComment {
			return ErrInvalidComment
		}
		category := strings.ToLower(strings.TrimSpace(request.ExpectedCategory))
		if status == commentModel.StatusApproved {
			if category != "" {
				return ErrInvalidComment
			}
		} else if !commentModeration.IsFeedbackCategory(category, s.config.CommentModerationSnapshot()) {
			return ErrInvalidComment
		}
		now := time.Now()
		feedback = &commentModel.CommentModerationFeedback{
			AuditID: auditID, OperatorID: operatorID, ExpectedStatus: status,
			ExpectedCategory: category, RelationCorrection: "{}", Note: strings.TrimSpace(request.Note),
			State: commentModel.ModerationFeedbackStateConfirmed, CreateTime: now, UpdateTime: now,
		}
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return fmt.Errorf("failed to load location: %w", err)
	}
	if err = s.commentModel.ReviewComment(ctx, commentID, status, time.Now().In(loc), feedback); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrCommentNotFound
		}
		return err
	}
	return nil
}

func (s *commentService) AdminDeleteComment(ctx context.Context, request *types.AdminDeleteCommentRequest) error {
	ids, err := parseAdminDeleteCommentIDs(request)
	if err != nil {
		s.logger.Error("invalid comment id", zap.Error(err))
		return ErrInvalidComment
	}

	items, err := s.commentModel.GetCommentsByIDs(ctx, ids)
	if err != nil {
		s.logger.Error("failed to get comments", zap.Error(err))
		return fmt.Errorf("failed to get comments: %w", err)
	}
	if len(items) != len(ids) {
		return ErrCommentNotFound
	}

	if err = s.commentModel.DeleteComments(ctx, ids); err != nil {
		s.logger.Error("failed to delete comments", zap.Error(err))
		return err
	}

	return nil
}

func (s *commentService) AdminPreviewCommentModeration(ctx context.Context,
	request *types.AdminPreviewCommentModerationRequest) (*types.AdminPreviewCommentModerationResponse, error) {

	comments, err := parseAdminPreviewComments(request)
	if err != nil {
		return nil, ErrInvalidComment
	}

	userID, err := parseOptionalAdminPreviewID("userID", request.UserID)
	if err != nil {
		s.logger.Error("invalid moderation preview user id", zap.Error(err))
		return nil, ErrInvalidComment
	}
	articleID, err := parseOptionalAdminPreviewID("articleID", request.ArticleID)
	if err != nil {
		s.logger.Error("invalid moderation preview article id", zap.Error(err))
		return nil, ErrInvalidComment
	}
	operatorID, err := parseOptionalAdminPreviewID("adminID", request.AdminID)
	if err != nil {
		s.logger.Error("invalid moderation preview admin id", zap.Error(err))
		return nil, ErrInvalidComment
	}

	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		s.logger.Error("failed to load location", zap.Error(err))
		return nil, fmt.Errorf("failed to load location: %w", err)
	}

	now := time.Now().In(loc)
	batchID := s.nextModerationBatchID(now)
	clientIP := strings.TrimSpace(request.ClientIP)
	response := &types.AdminPreviewCommentModerationResponse{
		BatchID:            batchID,
		EvaluatedAt:        now.Format(time.RFC3339Nano),
		PolicyVersion:      s.commentModerationPolicyVersion(),
		FeedbackCategories: commentModeration.FeedbackCategories(s.config.CommentModerationSnapshot()),
		Context: types.AdminCommentModerationExecutionContext{
			UserID:                  strings.TrimSpace(request.UserID),
			ArticleID:               strings.TrimSpace(request.ArticleID),
			ClientIPProvided:        clientIP != "",
			ArticleTitleProvided:    strings.TrimSpace(request.ArticleTitle) != "",
			ArticleCategoryProvided: strings.TrimSpace(request.ArticleCategory) != "",
			ParentContentProvided:   strings.TrimSpace(request.ParentContent) != "",
			ReplyContentProvided:    strings.TrimSpace(request.ReplyToContent) != "",
		},
		Rows:           make([]types.AdminPreviewCommentModerationItem, 0, len(comments)),
		Total:          len(comments),
		BehaviorDryRun: true,
	}
	audits := make([]commentModel.CommentModerationAudit, 0, len(comments))
	evaluateBehavior := userID != 0 || articleID != 0 || clientIP != ""
	for _, item := range comments {
		input := commentModerationInput{
			UserID:          userID,
			ArticleID:       articleID,
			ClientIP:        clientIP,
			Content:         item.content,
			ArticleTitle:    strings.TrimSpace(request.ArticleTitle),
			ArticleCategory: strings.TrimSpace(request.ArticleCategory),
			ParentContent:   strings.TrimSpace(request.ParentContent),
			ReplyToContent:  strings.TrimSpace(request.ReplyToContent),
			Now:             now,
		}
		var result commentModerationResult
		if evaluateBehavior {
			result = s.moderateComment(ctx, input)
		} else {
			result = s.moderateCommentWithBehavior(ctx, input, nil)
		}
		audit, auditErr := s.newCommentModerationAudit(commentModel.ModerationAuditSourceAdminSimulation,
			batchID, operatorID, input, result)
		if auditErr != nil {
			return nil, auditErr
		}
		audits = append(audits, audit)
		response.Rows = append(response.Rows, toAdminPreviewCommentModerationItem(item.line, item.content, result))
		incrementAdminPreviewSummary(response, result.Status)
	}
	if s.commentModel != nil {
		if err = s.commentModel.CreateModerationAudits(ctx, audits); err != nil {
			s.logger.Error("failed to create moderation simulation audits", zap.Error(err))
			return nil, err
		}
		for index := range response.Rows {
			response.Rows[index].AuditID = strconv.FormatUint(audits[index].ID, 10)
		}
	}

	return response, nil
}

func (s *commentService) AdminSubmitCommentModerationFeedback(ctx context.Context,
	request *types.AdminSubmitCommentModerationFeedbackRequest,
) error {
	if request == nil || s.commentModel == nil {
		return ErrInvalidComment
	}
	auditID, err := idutil.ParseID("auditID", request.AuditID)
	if err != nil {
		return ErrInvalidComment
	}
	operatorID, err := idutil.ParseID("adminID", request.AdminID)
	if err != nil {
		return ErrInvalidComment
	}
	if _, err = s.commentModel.GetModerationAuditByID(ctx, auditID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrCommentNotFound
		}
		return err
	}
	correction := "{}"
	if request.Relation != nil {
		encoded, encodeErr := json.Marshal(request.Relation)
		if encodeErr != nil {
			return ErrInvalidComment
		}
		correction = string(encoded)
	}
	now := time.Now()
	feedback := &commentModel.CommentModerationFeedback{
		AuditID:            auditID,
		OperatorID:         operatorID,
		ExpectedStatus:     strings.TrimSpace(request.ExpectedStatus),
		ExpectedCategory:   strings.ToLower(strings.TrimSpace(request.ExpectedCategory)),
		RelationCorrection: correction,
		Note:               strings.TrimSpace(request.Note),
		State:              commentModel.ModerationFeedbackStateConfirmed,
		CreateTime:         now,
		UpdateTime:         now,
	}
	if !commentModel.IsValidStatus(feedback.ExpectedStatus) {
		return ErrInvalidComment
	}
	if feedback.ExpectedStatus == commentModel.StatusApproved {
		if feedback.ExpectedCategory != "" {
			return ErrInvalidComment
		}
	} else if !commentModeration.IsFeedbackCategory(feedback.ExpectedCategory,
		s.config.CommentModerationSnapshot()) {
		return ErrInvalidComment
	}
	return s.commentModel.CreateModerationFeedback(ctx, feedback)
}

func toAdminCommentItem(row commentModel.AdminListItem) types.AdminCommentItem {
	item := types.AdminCommentItem{
		ID:                  strconv.FormatUint(row.ID, 10),
		ArticleTitle:        row.ArticleTitle,
		ReplyToAuthorName:   row.ReplyToAuthorName,
		ReplyToAuthorHandle: row.ReplyToAuthorHandle,
		AuthorHandle:        row.AuthorHandle,
		Content:             row.Content,
		Status:              row.Status,
		IP:                  row.IP,
		CreateTime:          row.CreateTime.Format(constants.TimeLayoutToMinute),
		UpdateTime:          row.UpdateTime.Format(constants.TimeLayoutToMinute),
	}
	if row.ParentID != 0 {
		item.ParentID = strconv.FormatUint(row.ParentID, 10)
	}
	if row.Status == commentModel.StatusPending || row.Status == commentModel.StatusRejected {
		item.ModerationReasons = formatCommentModerationReasons(decodeCommentModerationReasons(row.ModerationReasons))
	}
	return item
}

func parseOptionalAdminPreviewID(name, value string) (uint64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	return idutil.ParseID(name, value)
}

type adminPreviewCommentInput struct {
	line    int
	content string
}

func parseAdminPreviewComments(request *types.AdminPreviewCommentModerationRequest) ([]adminPreviewCommentInput, error) {
	if request == nil {
		return nil, errors.New("nil moderation preview request")
	}

	rawComments := request.Comments
	if len(rawComments) == 0 && strings.TrimSpace(request.Content) != "" {
		rawComments = []string{request.Content}
	}
	if len(rawComments) == 0 {
		return nil, errors.New("empty moderation preview comments")
	}

	comments := make([]adminPreviewCommentInput, 0, len(rawComments))
	for index, raw := range rawComments {
		content := strings.TrimSpace(raw)
		if content == "" {
			continue
		}
		if len([]rune(content)) > 1000 {
			return nil, errors.New("moderation preview comment too long")
		}
		comments = append(comments, adminPreviewCommentInput{
			line:    index + 1,
			content: content,
		})
		if len(comments) > 5000 {
			return nil, errors.New("too many moderation preview comments")
		}
	}
	if len(comments) == 0 {
		return nil, errors.New("empty moderation preview comments")
	}
	return comments, nil
}

func toAdminPreviewCommentModerationItem(line int, content string,
	result commentModerationResult) types.AdminPreviewCommentModerationItem {

	text := commentModeration.Normalize(content)
	return types.AdminPreviewCommentModerationItem{
		Line:       line,
		Content:    content,
		Status:     result.Status,
		RiskScore:  result.Score,
		Decision:   result.Decision,
		Reasons:    formatCommentModerationReasons(result.Reasons),
		RawReasons: compactCommentModerationReasons(result.Reasons),
		Signals:    toAdminCommentModerationSignals(result.Signals),
		Text:       toAdminCommentModerationTextView(text),
		Trace:      toAdminCommentModerationTrace(result.Trace),
	}
}

func incrementAdminPreviewSummary(response *types.AdminPreviewCommentModerationResponse, status string) {
	if response == nil {
		return
	}
	switch status {
	case commentModel.StatusApproved:
		response.Approved++
	case commentModel.StatusRejected:
		response.Rejected++
	default:
		response.Pending++
	}
}

func toAdminCommentModerationTextView(text commentModerationTextView) types.AdminCommentModerationTextView {
	return types.AdminCommentModerationTextView{
		Raw:          text.Raw,
		Normalized:   text.Normalized,
		Compact:      text.Compact,
		Confusable:   text.Confusable,
		DecodedTexts: append([]string{}, text.DecodedTexts...),
	}
}

func toAdminCommentModerationSignals(signals []commentModerationSignal) []types.AdminCommentModerationSignal {
	if len(signals) == 0 {
		return nil
	}
	values := make([]types.AdminCommentModerationSignal, 0, len(signals))
	for _, signal := range signals {
		values = append(values, types.AdminCommentModerationSignal{
			Source:     signal.Source,
			Category:   signal.Category,
			Level:      signal.Level,
			Score:      signal.Score,
			Reason:     signal.Reason,
			ReasonText: formatCommentModerationReason(signal.Reason),
			Evidence:   signal.Evidence,
			RuleID:     signal.RuleID,
			ClauseID:   signal.Clause,
		})
	}
	return values
}

func toAdminCommentModerationTrace(trace commentModeration.Trace) types.AdminCommentModerationTrace {
	clauses := make([]types.AdminCommentModerationClause, 0, len(trace.Clauses))
	for _, clause := range trace.Clauses {
		clauses = append(clauses, types.AdminCommentModerationClause{
			ID:   clause.ID,
			Text: toAdminCommentModerationTextView(clause.Text),
		})
	}
	return types.AdminCommentModerationTrace{
		Clauses:           clauses,
		DetectorSignals:   toAdminCommentModerationSignals(trace.DetectorSignals),
		SuppressedSignals: toAdminCommentModerationSignals(trace.SuppressedSignals),
		Behavior:          toAdminCommentModerationBehaviorTrace(trace.Behavior),
		DecisionEngine:    toAdminCommentModerationDecisionEngineTrace(trace.DecisionEngine),
		Decisions:         toAdminCommentModerationDecisionFlowTrace(trace.Decisions),
	}
}

func toAdminCommentModerationBehaviorTrace(
	trace commentModeration.BehaviorTrace,
) types.AdminCommentModerationBehaviorTrace {
	metrics := make([]types.AdminCommentModerationBehaviorMetric, 0, len(trace.Metrics))
	for _, item := range trace.Metrics {
		metrics = append(metrics, types.AdminCommentModerationBehaviorMetric{
			Name: item.Name, Evaluated: item.Evaluated,
			ObservedCount: item.ObservedCount, ProspectiveCount: item.ProspectiveCount,
			WindowSeconds: item.WindowSeconds, ReviewThreshold: item.ReviewThreshold,
			BlockThreshold: item.BlockThreshold, TriggeredLevel: item.TriggeredLevel,
			SkippedReason: item.SkippedReason,
		})
	}
	return types.AdminCommentModerationBehaviorTrace{
		Status: trace.Status, ReadOnly: trace.ReadOnly, ContextProvided: trace.ContextProvided,
		UnavailableReason: trace.UnavailableReason, Metrics: metrics,
	}
}

func toAdminCommentModerationDecisionEngineTrace(
	trace *commentModeration.DecisionEngineTrace,
) *types.AdminCommentModerationDecisionEngineTrace {
	if trace == nil {
		return nil
	}
	return &types.AdminCommentModerationDecisionEngineTrace{
		Candidates: toAdminCommentModerationRewriteCandidates(trace.Candidates),
		Evidence:   toAdminCommentModerationEvidence(trace.Evidence),
		Context: types.AdminCommentModerationContextAssessment{
			Analyzed:          trace.Context.Analyzed,
			Confidence:        trace.Context.Confidence,
			Intent:            trace.Context.Intent,
			BenignProbability: trace.Context.BenignProbability,
			Evidence:          toAdminCommentModerationEvidence(trace.Context.Evidence),
			Relations:         toAdminCommentModerationRelations(trace.Context.Relations),
			Explanation:       trace.Context.Explanation,
			UnavailableReason: trace.Context.UnavailableReason,
		},
		Fusion: toAdminCommentModerationEvidenceFusion(trace.Fusion),
		Decision: types.AdminCommentModerationProbabilityDecision{
			Status:                trace.Decision.Status,
			RiskProbability:       trace.Decision.RiskProbability,
			Confidence:            trace.Decision.Confidence,
			Decision:              trace.Decision.Decision,
			Calibration:           trace.Decision.Calibration,
			CategoryProbabilities: trace.Decision.CategoryProbabilities,
			Actionable:            trace.Decision.Actionable,
			FallbackReason:        trace.Decision.FallbackReason,
		},
	}
}

func toAdminCommentModerationEvidenceFusion(
	trace commentModeration.EvidenceFusionTrace,
) types.AdminCommentModerationEvidenceFusion {
	categories := make([]types.AdminCommentModerationCategoryFusion, 0, len(trace.Categories))
	for _, item := range trace.Categories {
		categories = append(categories, types.AdminCommentModerationCategoryFusion{
			Category: item.Category, RuleRisk: item.RuleRisk, ContextRisk: item.ContextRisk,
			ContextCovered: item.ContextCovered, ContextWeight: item.ContextWeight,
			ContentRisk: item.ContentRisk, BehaviorRisk: item.BehaviorRisk,
			CounterEvidence: item.CounterEvidence, FinalRisk: item.FinalRisk,
		})
	}
	deduplicated := make([]types.AdminCommentModerationEvidenceDeduplication, 0, len(trace.Deduplicated))
	for _, item := range trace.Deduplicated {
		deduplicated = append(deduplicated, types.AdminCommentModerationEvidenceDeduplication{
			Discarded: toAdminCommentModerationEvidenceItem(item.Discarded),
			KeptID:    item.KeptID, Reason: item.Reason,
		})
	}
	return types.AdminCommentModerationEvidenceFusion{
		Thresholds: types.AdminCommentModerationProbabilityThresholds{
			ApproveMax: trace.Thresholds.ApproveMax, RejectMin: trace.Thresholds.RejectMin,
			MinConfidence: trace.Thresholds.MinConfidence,
		},
		Categories: categories, InputCount: trace.InputCount, OutputCount: trace.OutputCount,
		Deduplicated: deduplicated,
	}
}

func toAdminCommentModerationDecisionFlowTrace(
	trace commentModeration.DecisionFlowTrace,
) types.AdminCommentModerationDecisionFlowTrace {
	snapshot := func(item commentModeration.DecisionSnapshot) types.AdminCommentModerationDecisionSnapshot {
		return types.AdminCommentModerationDecisionSnapshot{
			Status: item.Status, Score: item.Score, Decision: item.Decision,
		}
	}
	return types.AdminCommentModerationDecisionFlowTrace{
		Rule: snapshot(trace.Rule),
		Probability: types.AdminCommentModerationDecisionApplication{
			Evaluated: trace.Probability.Evaluated, Applied: trace.Probability.Applied,
			Before: snapshot(trace.Probability.Before), Candidate: snapshot(trace.Probability.Candidate),
			After: snapshot(trace.Probability.After), Reason: trace.Probability.Reason,
		},
		HardSafety: types.AdminCommentModerationHardSafety{
			Evaluated: trace.HardSafety.Evaluated, Triggered: trace.HardSafety.Triggered,
			RuleID: trace.HardSafety.RuleID, Before: snapshot(trace.HardSafety.Before),
			After: snapshot(trace.HardSafety.After), Reason: trace.HardSafety.Reason,
		},
		Feedback: types.AdminCommentModerationFeedbackApplication{
			Evaluated: trace.Feedback.Evaluated, Matched: trace.Feedback.Matched,
			Consensus: trace.Feedback.Consensus, Applied: trace.Feedback.Applied,
			Scope: trace.Feedback.Scope, Support: trace.Feedback.Support, Total: trace.Feedback.Total,
			Conflicts: trace.Feedback.Conflicts, SimulationSupport: trace.Feedback.SimulationSupport,
			LiveSupport: trace.Feedback.LiveSupport, ExpectedStatus: trace.Feedback.ExpectedStatus,
			ExpectedCategory: trace.Feedback.ExpectedCategory, Before: snapshot(trace.Feedback.Before),
			After: snapshot(trace.Feedback.After), Reason: trace.Feedback.Reason,
		},
		Final: snapshot(trace.Final),
	}
}

func toAdminCommentModerationRelations(
	items []commentModeration.SemanticRelation,
) []types.AdminCommentModerationRelation {
	if len(items) == 0 {
		return nil
	}
	result := make([]types.AdminCommentModerationRelation, 0, len(items))
	for _, item := range items {
		result = append(result, types.AdminCommentModerationRelation{
			ID:         item.ID,
			ClauseID:   item.Clause,
			Type:       item.Type,
			Subject:    item.Subject,
			Action:     item.Action,
			Object:     item.Object,
			Predicate:  item.Predicate,
			Result:     item.Result,
			Stance:     item.Stance,
			Category:   item.Category,
			Subtype:    item.Subtype,
			Evidence:   item.Evidence,
			Negated:    item.Negated,
			Quoted:     item.Quoted,
			Reported:   item.Reported,
			Inferred:   item.Inferred,
			Confidence: item.Confidence,
		})
	}
	return result
}

func toAdminCommentModerationRewriteCandidates(
	items []commentModeration.RewriteCandidate,
) []types.AdminCommentModerationRewriteCandidate {
	if len(items) == 0 {
		return nil
	}
	result := make([]types.AdminCommentModerationRewriteCandidate, 0, len(items))
	for _, item := range items {
		result = append(result, types.AdminCommentModerationRewriteCandidate{
			Text:       item.Text,
			Observed:   item.Observed,
			Category:   item.Category,
			Role:       item.Role,
			Method:     item.Method,
			Confidence: item.Confidence,
			Ambiguous:  item.Ambiguous,
			Rationale:  item.Rationale,
			ClauseID:   item.Clause,
		})
	}
	return result
}

func toAdminCommentModerationEvidence(items []commentModeration.Evidence) []types.AdminCommentModerationEvidence {
	if len(items) == 0 {
		return nil
	}
	result := make([]types.AdminCommentModerationEvidence, 0, len(items))
	for _, item := range items {
		result = append(result, toAdminCommentModerationEvidenceItem(item))
	}
	return result
}

func toAdminCommentModerationEvidenceItem(item commentModeration.Evidence) types.AdminCommentModerationEvidence {
	return types.AdminCommentModerationEvidence{
		ID: item.ID, Source: item.Source, Category: item.Category, Polarity: item.Polarity,
		Confidence: item.Confidence, CorrelationGroup: item.CorrelationGroup,
		Value: item.Value, RuleID: item.RuleID, ClauseID: item.Clause,
	}
}

func parseAdminCommentTimeRange(startValue string, endValue string) (*time.Time, *time.Time, error) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load location: %w", err)
	}

	startTime, err := parseAdminCommentTime(startValue, loc)
	if err != nil {
		return nil, nil, err
	}
	endTime, err := parseAdminCommentTime(endValue, loc)
	if err != nil {
		return nil, nil, err
	}
	if startTime != nil && endTime != nil && endTime.Before(*startTime) {
		return nil, nil, errors.New("create end time before start time")
	}
	return startTime, endTime, nil
}

func parseAdminCommentTime(value string, loc *time.Location) (*time.Time, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, nil
	}
	parsed, err := time.ParseInLocation(constants.TimeLayoutToSecond, trimmed, loc)
	if err != nil {
		parsed, err = time.ParseInLocation(constants.TimeLayoutToMinute, trimmed, loc)
	}
	if err != nil {
		return nil, fmt.Errorf("invalid time %q: %w", value, err)
	}
	return &parsed, nil
}

func normalizeAdminAuthorHandle(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	number, err := strconv.ParseUint(trimmed, 10, 64)
	if err != nil {
		return trimmed
	}
	if number > 0 && number < 100000 {
		return fmt.Sprintf("%05d", number)
	}
	return strconv.FormatUint(number, 10)
}

func parseAdminDeleteCommentIDs(request *types.AdminDeleteCommentRequest) ([]uint64, error) {
	rawIDs := make([]string, 0, len(request.IDList)+1)
	if strings.TrimSpace(request.ID) != "" {
		rawIDs = append(rawIDs, request.ID)
	}
	rawIDs = append(rawIDs, request.IDList...)
	if len(rawIDs) == 0 {
		return nil, errors.New("empty comment id list")
	}

	ids := make([]uint64, 0, len(rawIDs))
	seen := make(map[uint64]struct{}, len(rawIDs))
	for _, rawID := range rawIDs {
		id, err := idutil.ParseID("commentID", strings.TrimSpace(rawID))
		if err != nil {
			return nil, err
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, errors.New("empty comment id list")
	}
	return ids, nil
}

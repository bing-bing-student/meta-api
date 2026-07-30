package comment

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	commentModel "meta-api/app/model/comment"
	userModel "meta-api/app/model/user"
	"meta-api/common/constants"
	"meta-api/common/idutil"
	"meta-api/common/types"
)

const defaultCommentReportThreshold int64 = 3

func (s *commentService) UserReportComment(ctx context.Context,
	request *types.UserReportCommentRequest) (*types.UserReportCommentResponse, error) {

	user, err := s.getActiveCommentUser(ctx, request.UserID, request.SessionVersion)
	if err != nil {
		return nil, err
	}

	commentID, err := idutil.ParseID("commentID", request.CommentID)
	if err != nil {
		s.logger.Error("invalid comment id", zap.Error(err))
		return nil, ErrInvalidComment
	}

	item, err := s.commentModel.GetCommentByID(ctx, commentID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCommentNotFound
		}
		s.logger.Error("failed to get reported comment", zap.Error(err))
		return nil, fmt.Errorf("failed to get reported comment: %w", err)
	}
	if item.Status != commentModel.StatusApproved {
		return nil, ErrInvalidComment
	}
	if item.UserID == user.ID {
		return nil, ErrCommentForbidden
	}

	if _, err = s.commentModel.GetCommentReportByCommentAndReporter(ctx, commentID, user.ID); err == nil {
		return nil, ErrCommentAlreadyReported
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		s.logger.Error("failed to get comment report", zap.Error(err))
		return nil, fmt.Errorf("failed to get comment report: %w", err)
	}

	if err = s.checkCommentReportLimit(ctx, user.ID, commentID, request.ClientIP); err != nil {
		return nil, err
	}

	reportID, err := s.idGenerator.NextID()
	if err != nil {
		s.logger.Error("generate comment report id error", zap.Error(err))
		return nil, fmt.Errorf("generate comment report id error: %w", err)
	}
	now, err := commentServiceNow()
	if err != nil {
		s.logger.Error("failed to load location", zap.Error(err))
		return nil, err
	}

	report := &commentModel.CommentReport{
		ID:         reportID,
		CommentID:  commentID,
		ArticleID:  item.ArticleID,
		ReporterID: user.ID,
		Reason:     truncateString(strings.TrimSpace(request.Reason), 200),
		Status:     commentModel.ReportStatusPending,
		IP:         request.ClientIP,
		CreateTime: now,
		UpdateTime: now,
	}
	count, movedToPending, err := s.commentModel.CreateCommentReport(ctx, report, s.commentReportThreshold(), now)
	if err != nil {
		s.logger.Error("failed to create comment report", zap.Error(err))
		return nil, err
	}
	status := item.Status
	if movedToPending {
		status = commentModel.StatusPending
		if err = s.invalidateArticleCommentCache(ctx, item.ArticleID); err != nil {
			return nil, err
		}
	}

	return &types.UserReportCommentResponse{
		CommentID:   strconv.FormatUint(commentID, 10),
		ReportCount: count,
		Status:      status,
	}, nil
}

func (s *commentService) UserGetCommentReportStatus(ctx context.Context,
	request *types.UserGetCommentReportStatusRequest) (*types.UserGetCommentReportStatusResponse, error) {

	user, err := s.getActiveCommentUser(ctx, request.UserID, request.SessionVersion)
	if err != nil {
		return nil, err
	}

	commentIDs := make([]uint64, 0, len(request.CommentIDs))
	seen := make(map[uint64]struct{}, len(request.CommentIDs))
	for _, rawCommentID := range request.CommentIDs {
		commentID, parseErr := idutil.ParseID("commentID", rawCommentID)
		if parseErr != nil {
			s.logger.Error("invalid comment id", zap.Error(parseErr))
			return nil, ErrInvalidComment
		}
		if _, ok := seen[commentID]; ok {
			continue
		}
		seen[commentID] = struct{}{}
		commentIDs = append(commentIDs, commentID)
	}
	if len(commentIDs) == 0 {
		return &types.UserGetCommentReportStatusResponse{ReportedCommentIDs: []string{}}, nil
	}

	reportedIDs, err := s.commentModel.ListReportedCommentIDsByReporter(ctx, commentIDs, user.ID)
	if err != nil {
		s.logger.Error("failed to list reported comment ids", zap.Error(err))
		return nil, err
	}
	responseIDs := make([]string, 0, len(reportedIDs))
	for _, id := range reportedIDs {
		responseIDs = append(responseIDs, strconv.FormatUint(id, 10))
	}
	return &types.UserGetCommentReportStatusResponse{ReportedCommentIDs: responseIDs}, nil
}

func (s *commentService) AdminGetCommentReportList(ctx context.Context,
	request *types.AdminGetCommentReportListRequest) (*types.AdminGetCommentReportListResponse, error) {

	status := strings.TrimSpace(request.Status)
	if status != "" && !commentModel.IsValidReportStatus(status) {
		return nil, ErrInvalidComment
	}

	filter := commentModel.AdminReportListFilter{
		CommentQuery:   strings.TrimSpace(request.CommentQuery),
		AuthorHandle:   normalizeAdminAuthorHandle(request.AuthorHandle),
		ReporterHandle: normalizeAdminAuthorHandle(request.ReporterHandle),
		Status:         status,
		Offset:         (request.Page - 1) * request.PageSize,
		Limit:          request.PageSize,
	}
	rows, total, err := s.commentModel.ListCommentReports(ctx, filter)
	if err != nil {
		s.logger.Error("failed to list comment reports", zap.Error(err))
		return nil, err
	}

	responseRows := make([]types.AdminCommentReportItem, 0, len(rows))
	for _, row := range rows {
		responseRows = append(responseRows, toAdminCommentReportItem(row))
	}
	return &types.AdminGetCommentReportListResponse{
		Rows:  responseRows,
		Total: int(total),
	}, nil
}

func (s *commentService) AdminHandleCommentReport(ctx context.Context,
	request *types.AdminHandleCommentReportRequest) error {

	commentID, err := idutil.ParseID("commentID", request.CommentID)
	if err != nil {
		s.logger.Error("invalid comment id", zap.Error(err))
		return ErrInvalidComment
	}

	item, err := s.commentModel.GetCommentByID(ctx, commentID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrCommentNotFound
		}
		s.logger.Error("failed to get reported comment", zap.Error(err))
		return fmt.Errorf("failed to get reported comment: %w", err)
	}

	reportStatus, commentStatus, err := resolveCommentReportAction(request.Action)
	if err != nil {
		return ErrInvalidComment
	}
	now, err := commentServiceNow()
	if err != nil {
		s.logger.Error("failed to load location", zap.Error(err))
		return err
	}
	if err = s.commentModel.ResolvePendingCommentReports(ctx, commentID, reportStatus, commentStatus, now); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrCommentNotFound
		}
		s.logger.Error("failed to resolve comment reports", zap.Error(err))
		return err
	}
	return s.invalidateArticleCommentCache(ctx, item.ArticleID)
}

func (s *commentService) getActiveCommentUser(ctx context.Context, rawUserID string,
	sessionVersion int64) (*userModel.User, error) {

	userID, err := idutil.ParseID("userID", rawUserID)
	if err != nil {
		s.logger.Error("invalid comment user id", zap.Error(err))
		return nil, ErrCommentUnauthorized
	}
	user, err := s.userModel.GetUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCommentUnauthorized
		}
		s.logger.Error("failed to get comment user", zap.Error(err))
		return nil, fmt.Errorf("failed to get comment user: %w", err)
	}
	if sessionVersion > 0 {
		if sessionVersion != user.SessionVersion {
			return nil, ErrCommentSessionInvalid
		}
	} else if user.SessionVersion > 1 {
		return nil, ErrCommentSessionInvalid
	}
	return user, nil
}

func (s *commentService) commentReportThreshold() int64 {
	if s == nil || s.config == nil {
		return defaultCommentReportThreshold
	}
	cfg := s.config.CommentModerationSnapshot()
	if cfg.ReportThreshold <= 0 {
		return defaultCommentReportThreshold
	}
	return cfg.ReportThreshold
}

func resolveCommentReportAction(action string) (string, string, error) {
	switch strings.TrimSpace(action) {
	case "accept":
		return commentModel.ReportStatusAccepted, commentModel.StatusRejected, nil
	case "reject":
		return commentModel.ReportStatusRejected, commentModel.StatusApproved, nil
	default:
		return "", "", ErrInvalidComment
	}
}

func toAdminCommentReportItem(row commentModel.AdminReportListItem) types.AdminCommentReportItem {
	item := types.AdminCommentReportItem{
		ID:                  strconv.FormatUint(row.ID, 10),
		CommentID:           strconv.FormatUint(row.CommentID, 10),
		ArticleID:           strconv.FormatUint(row.ArticleID, 10),
		ArticleTitle:        row.ArticleTitle,
		CommentAuthorID:     strconv.FormatUint(row.CommentAuthorID, 10),
		CommentAuthorName:   row.CommentAuthorName,
		CommentAuthorHandle: row.CommentAuthorHandle,
		CommentContent:      row.CommentContent,
		CommentStatus:       row.CommentStatus,
		ReporterID:          strconv.FormatUint(row.ReporterID, 10),
		ReporterName:        row.ReporterName,
		ReporterHandle:      row.ReporterHandle,
		Reason:              row.Reason,
		Status:              row.Status,
		IP:                  row.IP,
		CreateTime:          row.CreateTime.Format(constants.TimeLayoutToMinute),
		UpdateTime:          row.UpdateTime.Format(constants.TimeLayoutToMinute),
	}
	if row.CommentAuthorID == 0 {
		item.CommentAuthorID = ""
	}
	return item
}

func commentServiceNow() (time.Time, error) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to load location: %w", err)
	}
	return time.Now().In(loc), nil
}

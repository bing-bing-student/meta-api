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
	"meta-api/common/cachekey"
	"meta-api/common/constants"
	"meta-api/common/idutil"
	"meta-api/common/types"
)

func (s *commentService) AdminGetCommentList(ctx context.Context,
	request *types.AdminGetCommentListRequest) (*types.AdminGetCommentListResponse, error) {

	filter := commentModel.AdminListFilter{
		ArticleTitle: strings.TrimSpace(request.ArticleTitle),
		AuthorHandle: normalizeAdminAuthorHandle(request.AuthorHandle),
		Status:       request.Status,
		Offset:       (request.Page - 1) * request.PageSize,
		Limit:        request.PageSize,
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

func (s *commentService) AdminUpdateCommentStatus(ctx context.Context,
	request *types.AdminUpdateCommentStatusRequest) error {

	id, err := idutil.ParseID("commentID", request.ID)
	if err != nil {
		s.logger.Error("invalid comment id", zap.Error(err))
		return ErrInvalidComment
	}

	item, err := s.commentModel.GetCommentByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrCommentNotFound
		}
		s.logger.Error("failed to get comment", zap.Error(err))
		return fmt.Errorf("failed to get comment: %w", err)
	}

	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		s.logger.Error("failed to load location", zap.Error(err))
		return fmt.Errorf("failed to load location: %w", err)
	}
	if err = s.commentModel.UpdateCommentStatus(ctx, id, request.Status, time.Now().In(loc)); err != nil {
		s.logger.Error("failed to update comment status", zap.Error(err))
		return err
	}

	return s.invalidateArticleCommentCache(ctx, item.ArticleID)
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

	articleIDSet := make(map[uint64]struct{}, len(items))
	for _, item := range items {
		articleIDSet[item.ArticleID] = struct{}{}
	}
	for articleID := range articleIDSet {
		if err = s.invalidateArticleCommentCache(ctx, articleID); err != nil {
			return err
		}
	}
	return nil
}

func (s *commentService) invalidateArticleCommentCache(ctx context.Context, articleID uint64) error {
	key := cachekey.CommentArticleApprovedList(strconv.FormatUint(articleID, 10)).String()
	if err := s.redis.Del(ctx, key).Err(); err != nil {
		s.logger.Error("failed to delete comment cache", zap.String("key", key), zap.Error(err))
		return fmt.Errorf("failed to delete comment cache: %w", err)
	}
	return nil
}

func toAdminCommentItem(row commentModel.AdminListItem) types.AdminCommentItem {
	item := types.AdminCommentItem{
		ID:                  strconv.FormatUint(row.ID, 10),
		ArticleID:           strconv.FormatUint(row.ArticleID, 10),
		ArticleTitle:        row.ArticleTitle,
		UserID:              strconv.FormatUint(row.UserID, 10),
		ReplyToAuthorName:   row.ReplyToAuthorName,
		ReplyToAuthorHandle: row.ReplyToAuthorHandle,
		AuthorName:          row.AuthorName,
		AuthorHandle:        row.AuthorHandle,
		AvatarURL:           row.AvatarURL,
		Provider:            row.Provider,
		Content:             row.Content,
		Status:              row.Status,
		IP:                  row.IP,
		CreateTime:          row.CreateTime.Format(constants.TimeLayoutToMinute),
		UpdateTime:          row.UpdateTime.Format(constants.TimeLayoutToMinute),
	}
	if row.ParentID != 0 {
		item.ParentID = strconv.FormatUint(row.ParentID, 10)
	}
	if row.ReplyToUserID != 0 {
		item.ReplyToUserID = strconv.FormatUint(row.ReplyToUserID, 10)
	}
	return item
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

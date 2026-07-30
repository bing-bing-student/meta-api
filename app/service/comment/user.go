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
	"meta-api/common/constants"
	"meta-api/common/idutil"
	"meta-api/common/types"
)

const (
	initialReplyPageSize          = 10
	replyToContentExcerptMaxRunes = 48
)

func (s *commentService) UserGetCommentList(ctx context.Context,
	request *types.UserGetCommentListRequest) (*types.UserGetCommentListResponse, error) {

	articleID, err := idutil.ParseID("articleID", request.ArticleID)
	if err != nil {
		s.logger.Error("invalid article id", zap.Error(err))
		return nil, ErrInvalidComment
	}

	start := (request.Page - 1) * request.PageSize
	parentRows, total, err := s.commentModel.ListApprovedParentsByArticleID(ctx, articleID, start, request.PageSize)
	if err != nil {
		return nil, err
	}

	rows := make([]types.UserCommentItem, 0, len(parentRows))
	for _, row := range parentRows {
		item := toUserCommentItem(row)
		if err = s.attachReplyPage(ctx, &item, row.ID, 1, initialReplyPageSize); err != nil {
			return nil, err
		}
		rows = append(rows, item)
	}

	return &types.UserGetCommentListResponse{
		Rows:  rows,
		Total: int(total),
	}, nil
}

func (s *commentService) UserGetCommentReplyList(ctx context.Context,
	request *types.UserGetCommentReplyListRequest) (*types.UserGetCommentReplyListResponse, error) {

	parentID, err := idutil.ParseID("parentID", request.ParentID)
	if err != nil {
		s.logger.Error("invalid parent comment id", zap.Error(err))
		return nil, ErrInvalidComment
	}

	parent, err := s.commentModel.GetCommentByID(ctx, parentID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCommentNotFound
		}
		s.logger.Error("failed to get parent comment", zap.Error(err))
		return nil, fmt.Errorf("failed to get parent comment: %w", err)
	}
	if parent.ParentID != 0 || parent.Status != commentModel.StatusApproved {
		return nil, ErrInvalidComment
	}

	start := (request.Page - 1) * request.PageSize
	replyRows, total, err := s.commentModel.ListApprovedRepliesByParentID(ctx, parentID, start, request.PageSize)
	if err != nil {
		return nil, err
	}

	rows := make([]types.UserCommentItem, 0, len(replyRows))
	for _, row := range replyRows {
		rows = append(rows, toUserCommentItem(row))
	}

	hasMore := request.Page*request.PageSize < int(total)
	nextPage := 0
	if hasMore {
		nextPage = request.Page + 1
	}
	return &types.UserGetCommentReplyListResponse{
		Rows:     rows,
		Total:    int(total),
		HasMore:  hasMore,
		NextPage: nextPage,
	}, nil
}

func (s *commentService) attachReplyPage(ctx context.Context, item *types.UserCommentItem, parentID uint64, page int, pageSize int) error {
	start := (page - 1) * pageSize
	replyRows, total, err := s.commentModel.ListApprovedRepliesByParentID(ctx, parentID, start, pageSize)
	if err != nil {
		return err
	}
	if len(replyRows) > 0 {
		item.Replies = make([]types.UserCommentItem, 0, len(replyRows))
		for _, row := range replyRows {
			item.Replies = append(item.Replies, toUserCommentItem(row))
		}
	}
	if page*pageSize < int(total) {
		item.ReplyHasMore = true
		item.ReplyNextPage = page + 1
	}
	return nil
}

func (s *commentService) UserAddComment(ctx context.Context,
	request *types.UserAddCommentRequest) (*types.UserAddCommentResponse, error) {

	userID, err := idutil.ParseID("userID", request.UserID)
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
	if request.SessionVersion > 0 {
		if request.SessionVersion != user.SessionVersion {
			return nil, ErrCommentSessionInvalid
		}
	} else if user.SessionVersion > 1 {
		return nil, ErrCommentSessionInvalid
	}
	if user.IsCommentDisabled(time.Now()) {
		return nil, ErrCommentForbidden
	}

	articleID, err := idutil.ParseID("articleID", request.ArticleID)
	if err != nil {
		s.logger.Error("invalid article id", zap.Error(err))
		return nil, ErrInvalidComment
	}
	if err = s.checkCommentSubmitLimit(ctx, userID, articleID, request.ClientIP); err != nil {
		return nil, err
	}
	if _, err = s.articleModel.GetArticleDetailByID(ctx, articleID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCommentNotFound
		}
		s.logger.Error("failed to get article for comment", zap.Error(err))
		return nil, fmt.Errorf("failed to get article: %w", err)
	}

	parentID := uint64(0)
	replyToUserID := uint64(0)
	replyToCommentID := uint64(0)
	if strings.TrimSpace(request.ParentID) != "" {
		parentID, err = idutil.ParseID("parentID", request.ParentID)
		if err != nil {
			s.logger.Error("invalid parent comment id", zap.Error(err))
			return nil, ErrInvalidComment
		}
		parent, err := s.commentModel.GetCommentByID(ctx, parentID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, ErrCommentNotFound
			}
			s.logger.Error("failed to get parent comment", zap.Error(err))
			return nil, fmt.Errorf("failed to get parent comment: %w", err)
		}
		if parent.ArticleID != articleID {
			return nil, ErrInvalidComment
		}
		if parent.Status != commentModel.StatusApproved {
			return nil, ErrInvalidComment
		}
		replyToCommentID = parent.ID
		replyToUserID = parent.UserID
		if parent.ParentID != 0 {
			parentID = parent.ParentID
		}
	}
	if strings.TrimSpace(request.ReplyToCommentID) != "" {
		replyToCommentID, err = idutil.ParseID("replyToCommentID", request.ReplyToCommentID)
		if err != nil {
			s.logger.Error("invalid reply target comment id", zap.Error(err))
			return nil, ErrInvalidComment
		}
		replyTarget, err := s.commentModel.GetCommentByID(ctx, replyToCommentID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, ErrCommentNotFound
			}
			s.logger.Error("failed to get reply target comment", zap.Error(err))
			return nil, fmt.Errorf("failed to get reply target comment: %w", err)
		}
		if replyTarget.ArticleID != articleID || replyTarget.Status != commentModel.StatusApproved {
			return nil, ErrInvalidComment
		}
		if parentID == 0 {
			if replyTarget.ParentID == 0 {
				parentID = replyTarget.ID
			} else {
				parentID = replyTarget.ParentID
			}
		}
		if replyTarget.ID != parentID && replyTarget.ParentID != parentID {
			return nil, ErrInvalidComment
		}
		replyToUserID = replyTarget.UserID
	}

	content := strings.TrimSpace(request.Content)
	if content == "" {
		return nil, ErrInvalidComment
	}

	commentID, err := s.idGenerator.NextID()
	if err != nil {
		s.logger.Error("generate comment id error", zap.Error(err))
		return nil, fmt.Errorf("generate comment id error: %w", err)
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		s.logger.Error("failed to load location", zap.Error(err))
		return nil, fmt.Errorf("failed to load location: %w", err)
	}
	now := time.Now().In(loc)
	moderationInput := commentModerationInput{
		CommentID: commentID,
		UserID:    userID,
		ArticleID: articleID,
		ClientIP:  request.ClientIP,
		Content:   content,
		Now:       now,
	}
	moderation := s.moderateComment(ctx, moderationInput)
	commentInfo := &commentModel.Comment{
		ID:               commentID,
		ArticleID:        articleID,
		ParentID:         parentID,
		UserID:           userID,
		ReplyToUserID:    replyToUserID,
		ReplyToCommentID: replyToCommentID,
		AuthorName:       truncateString(user.DisplayName, 80),
		Content:          content,
		Status:           moderation.Status,
		IP:               request.ClientIP,
		CreateTime:       now,
		UpdateTime:       now,
	}
	if err = s.commentModel.CreateComment(ctx, commentInfo); err != nil {
		s.logger.Error("failed to create comment", zap.Error(err))
		return nil, err
	}
	s.recordCommentModerationBehavior(ctx, moderationInput)

	return &types.UserAddCommentResponse{
		ID:     strconv.FormatUint(commentID, 10),
		Status: moderation.Status,
	}, nil
}

func toUserCommentItem(row commentModel.ListItem) types.UserCommentItem {
	item := types.UserCommentItem{
		ID:           strconv.FormatUint(row.ID, 10),
		ArticleID:    strconv.FormatUint(row.ArticleID, 10),
		UserID:       strconv.FormatUint(row.UserID, 10),
		AuthorName:   row.AuthorName,
		AuthorHandle: row.AuthorHandle,
		AvatarURL:    row.AvatarURL,
		Content:      row.Content,
		CreateTime:   row.CreateTime.Format(constants.TimeLayoutToMinute),
	}
	if row.ParentID != 0 {
		item.ParentID = strconv.FormatUint(row.ParentID, 10)
	}
	if row.ReplyToUserID != 0 {
		item.ReplyToUserID = strconv.FormatUint(row.ReplyToUserID, 10)
		item.ReplyToAuthorName = row.ReplyToAuthorName
		item.ReplyToAuthorHandle = row.ReplyToAuthorHandle
	}
	if row.ReplyToCommentID != 0 {
		item.ReplyToCommentID = strconv.FormatUint(row.ReplyToCommentID, 10)
		item.ReplyToContentExcerpt = buildCommentExcerpt(row.ReplyToContent)
	}
	return item
}

func truncateString(value string, maxLen int) string {
	runes := []rune(value)
	if len(runes) <= maxLen {
		return value
	}
	return string(runes[:maxLen])
}

func buildCommentExcerpt(value string) string {
	content := strings.Join(strings.Fields(value), " ")
	runes := []rune(content)
	if len(runes) <= replyToContentExcerptMaxRunes {
		return content
	}
	return string(runes[:replyToContentExcerptMaxRunes]) + "..."
}

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

// UserGetCommentList 校验 request 中的文章 ID，并读取该文章已通过的一级评论及首屏回复。
// 输入 ctx 控制缓存和数据库操作；返回分页响应，ID 非法时返回 ErrInvalidComment，读取失败时返回底层错误。
func (s *commentService) UserGetCommentList(ctx context.Context,
	request *types.UserGetCommentListRequest) (*types.UserGetCommentListResponse, error) {

	articleID, err := idutil.ParseID("articleID", request.ArticleID)
	if err != nil {
		s.logger.Error("invalid article id", zap.Error(err))
		return nil, ErrInvalidComment
	}

	approvedComments, err := s.getApprovedArticleComments(ctx, articleID)
	if err != nil {
		return nil, err
	}
	return buildUserCommentListResponse(approvedComments, request.Page, request.PageSize), nil
}

// UserGetCommentReplyList 校验 request 指定的一级评论并返回其已通过回复分页。
// 输入 ctx 控制查询；返回回复响应，父评论不存在、不是一级评论或未通过审核时返回相应业务错误。
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

	approvedComments, err := s.getApprovedArticleComments(ctx, parent.ArticleID)
	if err != nil {
		return nil, err
	}
	return buildUserCommentReplyListResponse(approvedComments, strconv.FormatUint(parentID, 10),
		request.Page, request.PageSize), nil
}

// UserAddComment 完成用户会话、禁言、限流、文章、回复关系和内容校验，审核评论后将评论与审计记录原子落库。
// 输入 ctx 控制所有下游操作，request 携带身份、文章、回复目标及内容；返回评论 ID 和审核状态，失败时返回业务或存储错误。
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
	articleDetail, err := s.articleModel.GetArticleDetailByID(ctx, articleID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCommentNotFound
		}
		s.logger.Error("failed to get article for comment", zap.Error(err))
		return nil, fmt.Errorf("failed to get article: %w", err)
	}

	parentID := uint64(0)
	replyToUserID := uint64(0)
	replyToCommentID := uint64(0)
	parentContent := ""
	replyToContent := ""
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
		parentContent = parent.Content
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
		replyToContent = replyTarget.Content
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
		CommentID:       commentID,
		UserID:          userID,
		ArticleID:       articleID,
		ClientIP:        request.ClientIP,
		Content:         content,
		ArticleTitle:    articleDetail.Title,
		ArticleCategory: articleDetail.TagName,
		ParentContent:   parentContent,
		ReplyToContent:  replyToContent,
		Now:             now,
	}
	moderation := s.moderateComment(ctx, moderationInput)
	commentInfo := &commentModel.Comment{
		ID:                commentID,
		ArticleID:         articleID,
		ParentID:          parentID,
		UserID:            userID,
		ReplyToUserID:     replyToUserID,
		ReplyToCommentID:  replyToCommentID,
		AuthorName:        truncateString(user.DisplayName, 80),
		Content:           content,
		Status:            moderation.Status,
		ModerationReasons: encodeCommentModerationReasons(moderation.Reasons),
		IP:                request.ClientIP,
		CreateTime:        now,
		UpdateTime:        now,
	}
	audit, err := s.newCommentModerationAudit(commentModel.ModerationAuditSourceLiveComment, "", 0,
		moderationInput, moderation)
	if err != nil {
		s.logger.Error("failed to build comment moderation audit", zap.Error(err))
		return nil, err
	}
	if err = s.commentModel.CreateCommentWithModerationAudit(ctx, commentInfo, &audit); err != nil {
		s.logger.Error("failed to create comment", zap.Error(err))
		return nil, err
	}
	s.recordCommentModerationBehavior(ctx, moderationInput)
	if moderation.Status == commentModel.StatusApproved {
		if err = s.clearApprovedArticleCommentCache(ctx, articleID); err != nil {
			s.logger.Error("failed to clear approved comment cache after create",
				zap.Uint64("article_id", articleID), zap.Error(err))
		}
	}

	return &types.UserAddCommentResponse{
		ID:     strconv.FormatUint(commentID, 10),
		Status: moderation.Status,
	}, nil
}

// toUserCommentItem 将数据库列表行 row 转换为前台评论项。
// 返回值会格式化 ID、时间、回复对象，并为被回复评论生成受长度限制的摘要。
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

// truncateString 将 value 按 Unicode 字符数限制为 maxLen。
// 返回原字符串或截断结果；该函数按字符而非字节截断，避免破坏 UTF-8。
func truncateString(value string, maxLen int) string {
	runes := []rune(value)
	if len(runes) <= maxLen {
		return value
	}
	return string(runes[:maxLen])
}

// buildCommentExcerpt 将 value 的连续空白压缩为空格，并限制为回复摘要最大字符数。
// 返回清理后的完整文本或带省略号的截断摘要。
func buildCommentExcerpt(value string) string {
	content := strings.Join(strings.Fields(value), " ")
	runes := []rune(content)
	if len(runes) <= replyToContentExcerptMaxRunes {
		return content
	}
	return string(runes[:replyToContentExcerptMaxRunes]) + "..."
}

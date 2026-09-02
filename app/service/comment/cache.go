package comment

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/bytedance/sonic"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"meta-api/common/cachekey"
	"meta-api/common/types"
)

// getApprovedArticleComments 获取 articleID 对应的全部已通过评论，优先读取永久业务缓存，未命中时查询数据库并回填。
// 输入 ctx 用于 Redis 和数据库调用；返回按模型顺序转换的评论列表，数据库或序列化失败时返回错误，缓存故障会降级。
func (s *commentService) getApprovedArticleComments(ctx context.Context,
	articleID uint64) ([]types.UserCommentItem, error) {

	key := cachekey.CommentApprovedArticle(strconv.FormatUint(articleID, 10)).String()
	if s.redis != nil {
		value, err := s.redis.Get(ctx, key).Bytes()
		switch {
		case err == nil:
			items := make([]types.UserCommentItem, 0)
			if err = sonic.Unmarshal(value, &items); err == nil {
				return items, nil
			}
			s.logger.Warn("failed to decode approved comment cache",
				zap.String("key", key), zap.Error(err))
		case errors.Is(err, redis.Nil):
		default:
			s.logger.Warn("failed to read approved comment cache",
				zap.String("key", key), zap.Error(err))
		}
	}

	rows, err := s.commentModel.ListApprovedByArticleID(ctx, articleID)
	if err != nil {
		return nil, err
	}
	items := make([]types.UserCommentItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, toUserCommentItem(row))
	}

	if s.redis != nil {
		value, marshalErr := sonic.Marshal(items)
		if marshalErr != nil {
			return nil, fmt.Errorf("encode approved comments: %w", marshalErr)
		}
		// 业务缓存不设 TTL，评论变更时按文章主动失效。
		if setErr := s.redis.Set(ctx, key, value, 0).Err(); setErr != nil {
			s.logger.Warn("failed to cache approved comments",
				zap.String("key", key), zap.Error(setErr))
		}
	}
	return items, nil
}

// clearApprovedArticleCommentCache 删除 articleIDs 对应的已通过评论缓存。
// 输入 ctx 控制 Redis 操作，articleIDs 会过滤零值并去重；返回删除错误，未配置 Redis 或无有效文章时返回 nil。
func (s *commentService) clearApprovedArticleCommentCache(ctx context.Context,
	articleIDs ...uint64) error {

	if s.redis == nil || len(articleIDs) == 0 {
		return nil
	}
	seen := make(map[uint64]struct{}, len(articleIDs))
	keys := make([]string, 0, len(articleIDs))
	for _, articleID := range articleIDs {
		if articleID == 0 {
			continue
		}
		if _, exists := seen[articleID]; exists {
			continue
		}
		seen[articleID] = struct{}{}
		keys = append(keys, cachekey.CommentApprovedArticle(strconv.FormatUint(articleID, 10)).String())
	}
	if len(keys) == 0 {
		return nil
	}
	if err := s.redis.Del(ctx, keys...).Err(); err != nil {
		return fmt.Errorf("clear approved comment cache: %w", err)
	}
	return nil
}

// organizeComments 按 ParentID 将 items 拆分为一级评论和父评论到回复列表的映射。
// 返回值依次为保持原顺序的一级评论及每个父评论的回复集合。
func organizeComments(items []types.UserCommentItem) ([]types.UserCommentItem,
	map[string][]types.UserCommentItem) {

	parents := make([]types.UserCommentItem, 0)
	repliesByParent := make(map[string][]types.UserCommentItem)
	for _, item := range items {
		if item.ParentID == "" {
			parents = append(parents, item)
			continue
		}
		repliesByParent[item.ParentID] = append(repliesByParent[item.ParentID], item)
	}
	return parents, repliesByParent
}

// commentPage 根据从 1 开始的 page 和 pageSize 截取 items。
// 返回当前页切片；起始位置超出列表时返回非 nil 空切片。
func commentPage(items []types.UserCommentItem, page, pageSize int) []types.UserCommentItem {
	start := (page - 1) * pageSize
	if start >= len(items) {
		return []types.UserCommentItem{}
	}
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}
	return items[start:end]
}

// buildUserCommentListResponse 将全部已通过 items 组织为一级评论分页，并为每条一级评论附带首屏回复。
// 输入 page 和 pageSize 控制一级评论分页；返回包含总数、回复续页标记和下一页编号的响应。
func buildUserCommentListResponse(items []types.UserCommentItem,
	page, pageSize int) *types.UserGetCommentListResponse {

	parents, repliesByParent := organizeComments(items)
	rows := commentPage(parents, page, pageSize)
	for index := range rows {
		replies := repliesByParent[rows[index].ID]
		rows[index].Replies = commentPage(replies, 1, initialReplyPageSize)
		if len(rows[index].Replies) == 0 {
			rows[index].Replies = nil
		}
		if len(replies) > initialReplyPageSize {
			rows[index].ReplyHasMore = true
			rows[index].ReplyNextPage = 2
		}
	}
	return &types.UserGetCommentListResponse{Rows: rows, Total: len(parents)}
}

// buildUserCommentReplyListResponse 从 items 中提取 parentID 对应回复并分页。
// 输入 page 和 pageSize 控制回复页；返回回复列表、总数、是否还有下一页及下一页编号。
func buildUserCommentReplyListResponse(items []types.UserCommentItem, parentID string,
	page, pageSize int) *types.UserGetCommentReplyListResponse {

	_, repliesByParent := organizeComments(items)
	replies := repliesByParent[parentID]
	rows := commentPage(replies, page, pageSize)
	hasMore := page*pageSize < len(replies)
	nextPage := 0
	if hasMore {
		nextPage = page + 1
	}
	return &types.UserGetCommentReplyListResponse{
		Rows:     rows,
		Total:    len(replies),
		HasMore:  hasMore,
		NextPage: nextPage,
	}
}

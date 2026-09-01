package tag

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"meta-api/common/cachekey"
	"meta-api/common/constants"
	"meta-api/common/types"
)

// UserGetTagList 获取标签列表
func (t *tagService) UserGetTagList(ctx context.Context) (*types.UserGetTagListResponse, error) {
	response := &types.UserGetTagListResponse{}
	key := cachekey.TagArticleNumZSet().String()

	pipe := t.redis.Pipeline()
	rowsCmd := pipe.ZRevRangeWithScores(ctx, key, 0, -1)
	totalCmd := pipe.ZCard(ctx, key)
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, fmt.Errorf("failed to read tag cache: %w", err)
	}
	tagZSet, err := rowsCmd.Result()
	if err != nil {
		return nil, err
	}
	total, err := totalCmd.Result()
	if err != nil {
		return nil, err
	}
	if total == 0 {
		articleCountWithTagNameList, err := t.tagModel.GetArticleCountWithTagName(ctx)
		if err != nil {
			t.logger.Error("failed to get ArticleCountWithTag", zap.Error(err))
			return nil, fmt.Errorf("failed to get ArticleCountWithTag, err: %w", err)
		}
		if len(articleCountWithTagNameList) > 0 {
			zAddArgs := make([]redis.Z, len(articleCountWithTagNameList))
			for i, data := range articleCountWithTagNameList {
				zAddArgs[i] = redis.Z{
					Score:  float64(data.Count),
					Member: data.Name,
				}
				response.Rows = append(response.Rows, types.TagNameWithArticleNumItem{
					Name:       data.Name,
					ArticleNum: data.Count,
				})
			}

			// 批量写入 Redis
			if err = t.redis.ZAdd(ctx, key, zAddArgs...).Err(); err != nil {
				t.logger.Error("failed to write tag:articleNum:ZSet", zap.Error(err))
				return nil, fmt.Errorf("failed to write tag:articleNum:ZSet, err: %w", err)
			}
		}
		response.Total = len(response.Rows)
	} else {
		for _, label := range tagZSet {
			name, ok := label.Member.(string)
			if !ok {
				return nil, fmt.Errorf("invalid tag cache member type %T", label.Member)
			}
			response.Rows = append(response.Rows, types.TagNameWithArticleNumItem{
				Name:       name,
				ArticleNum: int(label.Score),
			})
		}
		response.Total = int(total)
	}

	return response, nil
}

// UserGetArticleListByTag 获取标签下的文章列表
func (t *tagService) UserGetArticleListByTag(ctx context.Context,
	request *types.UserGetArticleListByTagRequest) (*types.UserGetArticleListByTagResponse, error) {

	start := (request.Page - 1) * request.PageSize
	stop := start + request.PageSize - 1
	key := cachekey.TagArticleListZSet(request.TagName).String()
	response := &types.UserGetArticleListByTagResponse{}

	articleIDList, total, err := t.loadTagArticlePage(ctx, key, request.TagName, start, stop)
	if err != nil {
		t.logger.Error("failed to load tag article page", zap.Error(err))
		return nil, err
	}
	entries, misses, err := t.readTagArticleCache(ctx, articleIDList)
	if err != nil {
		return nil, err
	}
	loaded, err := t.loadTagArticleMisses(ctx, misses)
	if err != nil {
		return nil, err
	}
	for id, entry := range loaded {
		entries[id] = entry
	}
	for _, articleID := range articleIDList {
		entry, exists := entries[articleID]
		if !exists {
			return nil, fmt.Errorf("article %s not found", articleID)
		}
		response.Rows = append(response.Rows, types.UserGetArticleItem{
			ID:         articleID,
			Title:      entry.Title,
			Describe:   entry.Describe,
			CreateTime: formatTagArticleTime(entry.CreateTime, constants.TimeLayoutToDay),
			UpdateTime: formatTagArticleTime(entry.UpdateTime, constants.TimeLayoutToDay),
			ViewNum:    entry.ViewNum,
		})
	}
	response.Total = int(total)
	return response, nil
}

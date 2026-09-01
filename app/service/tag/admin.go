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

// AdminGetTagList 获取标签列表
func (t *tagService) AdminGetTagList(ctx context.Context) (*types.AdminGetTagListResponse, error) {
	response := &types.AdminGetTagListResponse{}
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

// AdminGetArticleListByTag 通过标签获取文章列表
func (t *tagService) AdminGetArticleListByTag(ctx context.Context,
	request *types.AdminGetArticleListByTagRequest) (*types.AdminGetArticleListByTagResponse, error) {

	start := (request.Page - 1) * request.PageSize
	stop := start + request.PageSize - 1
	key := cachekey.TagArticleListZSet(request.TagName).String()
	response := &types.AdminGetArticleListByTagResponse{}

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
		response.Rows = append(response.Rows, types.AdminGetArticleListByTagItem{
			ID:         articleID,
			Title:      entry.Title,
			ViewNum:    entry.ViewNum,
			CreateTime: formatTagArticleTime(entry.CreateTime, constants.TimeLayoutToMinute),
			UpdateTime: formatTagArticleTime(entry.UpdateTime, constants.TimeLayoutToMinute),
		})
	}
	response.Total = int(total)
	return response, nil
}

func formatTimeToMinute(value string) string {
	if len(value) <= len(constants.TimeLayoutToMinute) {
		return value
	}
	return value[:len(constants.TimeLayoutToMinute)]
}

// AdminUpdateTag 更新标签
func (t *tagService) AdminUpdateTag(ctx context.Context, request *types.AdminUpdateTagRequest) error {
	tagInfo, err := t.tagModel.FindTagByName(ctx, request.NewTagName)
	if err != nil {
		t.logger.Error("FindTagByName error", zap.Error(err))
		return fmt.Errorf("FindTagByName error: %w", err)
	}
	if tagInfo.ID == 0 {
		// 如果标签不存在，则需要插入新标签
		tagID, err := t.idGenerator.NextID()
		if err != nil {
			t.logger.Error("generate id error", zap.Error(err))
			return fmt.Errorf("generate id error: %w", err)
		}
		tagInfo.ID = tagID
		tagInfo.Name = request.NewTagName
		if err = t.tagModel.CreateTag(ctx, tagInfo); err != nil {
			t.logger.Error("failed to create new tag", zap.Error(err))
			return fmt.Errorf("failed to create new tag: %w", err)
		}
	}

	// 更新文章表中的标签 ID
	if err = t.articleModel.UpdateArticleTagID(ctx, request.ArticleIDList, tagInfo.ID); err != nil {
		t.logger.Error("failed to update article list tag", zap.Error(err))
		return fmt.Errorf("failed to update article list tag: %w", err)
	}

	// 更新标签之前先将缓存当中的浏览量数据写入 mysql
	for _, articleID := range request.ArticleIDList {
		viewNum, err := t.redis.ZScore(ctx, cachekey.ArticleViewZSet().String(), articleID).Result()
		if err != nil {
			t.logger.Error("failed to query article:view:ZSet", zap.Error(err))
			return fmt.Errorf("failed to query article:view:ZSet: %w", err)
		}
		if err = t.articleModel.UpdateArticleViewNum(ctx, articleID, viewNum); err != nil {
			t.logger.Error("failed to update article view num", zap.Error(err))
			return fmt.Errorf("failed to update article view num: %w", err)
		}
	}

	// 删除缓存脏数据
	for _, id := range request.ArticleIDList {
		if err = t.redis.Del(ctx, cachekey.ArticleHash(id).String()).Err(); err != nil {
			t.logger.Error("failed to delete article:id:Hash", zap.Error(err))
			return fmt.Errorf("failed to delete article:id:Hash: %w", err)
		}
	}

	if err = t.redis.Del(ctx, cachekey.TagArticleListZSet(request.OldTagName).String()).Err(); err != nil {
		t.logger.Error("failed to delete oldTagName:article:ZSet", zap.Error(err))
		return fmt.Errorf("failed to delete oldTagName:article:ZSet: %w", err)
	}

	if err = t.redis.Del(ctx, cachekey.TagArticleListZSet(request.NewTagName).String()).Err(); err != nil {
		t.logger.Error("failed to delete newTagName:article:ZSet", zap.Error(err))
		return fmt.Errorf("failed to delete newTagName:article:ZSet: %w", err)
	}

	if err = t.redis.Del(ctx, cachekey.TagArticleNumZSet().String()).Err(); err != nil {
		t.logger.Error("failed to delete tag:articleNum:ZSet", zap.Error(err))
		return fmt.Errorf("failed to delete tag:articleNum:ZSet: %w", err)
	}

	// 刷新 sitemap 内部缓存，让标签 URL 和文章归属变更尽快反映到 sitemap.xml。
	t.sitemap.RefreshArticles(request.ArticleIDList...)

	// 清理 CDN 上受影响文章详情 HTML 缓存，避免页面继续展示旧标签。
	if err = t.cdn.PurgeArticles(request.ArticleIDList...); err != nil {
		t.logger.Error("failed to purge article CDN cache",
			zap.Strings("article_ids", request.ArticleIDList), zap.Error(err))
		return fmt.Errorf("failed to purge article CDN cache: %w", err)
	}

	return nil
}

package tag

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"meta-api/common/constants"
	"meta-api/common/types"
)

// AdminGetTagList 获取标签列表
func (t *tagService) AdminGetTagList(ctx context.Context) (*types.AdminGetTagListResponse, error) {
	response := &types.AdminGetTagListResponse{}
	key := "tag:articleNum:ZSet"

	if exist := t.redis.Exists(ctx, key).Val(); exist == 0 {
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

			// 批量写入Redis
			if err = t.redis.ZAdd(ctx, key, zAddArgs...).Err(); err != nil {
				t.logger.Error("failed to write tag:articleNum:ZSet", zap.Error(err))
				return nil, fmt.Errorf("failed to write tag:articleNum:ZSet, err: %w", err)
			}
		}
	} else {
		// 获取Redis当中的标签数据(无分页)
		tagZSet, err := t.redis.ZRevRangeWithScores(ctx, key, 0, -1).Result()
		if err != nil {
			t.logger.Error("failed to get tag:articleNum:ZSet", zap.Error(err))
			return nil, fmt.Errorf("failed to get tag:articleNum:ZSet, err: %w", err)
		}

		for _, label := range tagZSet {
			response.Rows = append(response.Rows, types.TagNameWithArticleNumItem{
				Name:       label.Member.(string),
				ArticleNum: int(label.Score),
			})
		}
	}
	response.Total = int(t.redis.ZCard(ctx, key).Val())

	return response, nil
}

// AdminGetArticleListByTag 通过标签获取文章列表
func (t *tagService) AdminGetArticleListByTag(ctx context.Context,
	request *types.AdminGetArticleListByTagRequest) (*types.AdminGetArticleListByTagResponse, error) {

	start := (request.Page - 1) * request.PageSize
	stop := start + request.PageSize - 1
	key := request.TagName + ":article:ZSet"
	response := &types.AdminGetArticleListByTagResponse{}

	// 获取文章ID列表(包含分页条件)
	articleIDList, err := t.redis.ZRevRange(ctx, key, int64(start), int64(stop)).Result()
	if err != nil {
		t.logger.Error("failed to get article:ZSet", zap.Error(err))
		return nil, err
	}
	// 如果Redis中没有这个有序集合
	if len(articleIDList) == 0 {
		// 查询MySQL
		articleList, err := t.tagModel.GetArticleListByTagName(ctx, request.TagName)
		if err != nil {
			t.logger.Error("failed to get tagIDArticleZSet", zap.Error(err))
			return nil, err
		}
		if len(articleList) == 0 {
			t.logger.Error("not found tagName", zap.Error(err))
			return nil, fmt.Errorf("not found tagName")
		}
		// 写入Redis
		for _, v := range articleList {
			if err = t.redis.ZAdd(ctx, key, redis.Z{
				Score:  float64(v.CreateTime.UnixNano() / int64(time.Millisecond)),
				Member: v.ID,
			}).Err(); err != nil {
				t.logger.Error("failed to write article:ZSet", zap.Error(err))
				return nil, err
			}
		}
		// 再次获取数据(包含分页条件)
		articleIDList, err = t.redis.ZRevRange(ctx, key, int64(start), int64(stop)).Result()
		if err != nil {
			t.logger.Error("failed to get article:ZSet", zap.Error(err))
			return nil, err
		}
	}

	// 获取Redis当中的文章Hash数据
	fields := []string{"title", "viewNum", "createTime", "updateTime"}
	for _, articleID := range articleIDList {
		articleItem := types.AdminGetArticleListByTagItem{}

		// 如果Redis中没有这个文章Hash数据
		if exists := t.redis.Exists(ctx, "article:"+articleID+":Hash").Val(); exists == 0 {
			id, err := strconv.ParseUint(articleID, 10, 64)
			if err != nil {
				t.logger.Error("parse uint64 error", zap.Error(err))
				return response, err
			}
			articleInfo, mysqlErr := t.articleModel.GetArticleDetailByID(ctx, id)
			if mysqlErr != nil {
				t.logger.Error("failed to get article from MySQL", zap.Error(mysqlErr))
				return nil, mysqlErr
			}
			mapData := map[string]interface{}{
				"id":         articleInfo.ID,
				"title":      articleInfo.Title,
				"describe":   articleInfo.Describe,
				"content":    articleInfo.Content,
				"viewNum":    articleInfo.ViewNum,
				"createTime": articleInfo.CreateTime.Format(constants.TimeLayoutToSecond),
				"updateTime": articleInfo.UpdateTime.Format(constants.TimeLayoutToSecond),
				"tagID":      articleInfo.TagID,
				"tagName":    articleInfo.TagName,
			}
			if err = t.redis.HMSet(ctx, "article:"+articleID+":Hash", mapData).Err(); err != nil {
				t.logger.Error("failed to write article:articleID:ZSet", zap.Error(err))
				return nil, fmt.Errorf("failed to write article:articleID:ZSet: %w", err)
			}

			articleItem.ID = articleID
			articleItem.Title = articleInfo.Title
			articleItem.ViewNum = int(articleInfo.ViewNum)
			articleItem.CreateTime = articleInfo.CreateTime.Format(constants.TimeLayoutToMinute)
			articleItem.UpdateTime = articleInfo.UpdateTime.Format(constants.TimeLayoutToMinute)
		}

		// 获取缓存当中的文章详情
		data, err := t.redis.HMGet(ctx, "article:"+articleID+":Hash", fields...).Result()
		if err != nil {
			t.logger.Error("failed to get article:ZSet", zap.Error(err))
			return nil, err
		}
		articleItem.ID = articleID
		articleItem.Title = data[0].(string)
		viewNumStr := data[1].(string)
		articleItem.ViewNum, err = strconv.Atoi(viewNumStr)
		if err != nil {
			t.logger.Error("parse string to int error", zap.Error(err))
			return nil, fmt.Errorf("parse string to int error, err: %w", err)
		}
		articleItem.CreateTime = data[2].(string)
		articleItem.UpdateTime = data[3].(string)
		response.Rows = append(response.Rows, articleItem)
	}
	response.Total = int(t.redis.ZCard(ctx, key).Val())
	return response, nil
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

	// 更新文章表中的标签ID
	if err = t.articleModel.UpdateArticleTagID(ctx, request.ArticleIDList, tagInfo.ID); err != nil {
		t.logger.Error("failed to update article list tag", zap.Error(err))
		return fmt.Errorf("failed to update article list tag: %w", err)
	}

	// 更新标签之前先将缓存当中的浏览量数据写入mysql
	for _, articleID := range request.ArticleIDList {
		viewNum, err := t.redis.ZScore(ctx, "article:view:ZSet", articleID).Result()
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
		if err = t.redis.Del(ctx, "article:"+id+":Hash").Err(); err != nil {
			t.logger.Error("failed to delete article:id:Hash", zap.Error(err))
			return fmt.Errorf("failed to delete article:id:Hash: %w", err)
		}
	}

	if err = t.redis.Del(ctx, request.OldTagName+":article:ZSet").Err(); err != nil {
		t.logger.Error("failed to delete oldTagName:article:ZSet", zap.Error(err))
		return fmt.Errorf("failed to delete oldTagName:article:ZSet: %w", err)
	}

	if err = t.redis.Del(ctx, request.NewTagName+":article:ZSet").Err(); err != nil {
		t.logger.Error("failed to delete newTagName:article:ZSet", zap.Error(err))
		return fmt.Errorf("failed to delete newTagName:article:ZSet: %w", err)
	}

	if err = t.redis.Del(ctx, "tag:articleNum:ZSet").Err(); err != nil {
		t.logger.Error("failed to delete tag:articleNum:ZSet", zap.Error(err))
		return fmt.Errorf("failed to delete tag:articleNum:ZSet: %w", err)
	}

	return nil
}

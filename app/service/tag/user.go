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

// UserGetTagList 获取标签列表
func (t *tagService) UserGetTagList(ctx context.Context) (*types.UserGetTagListResponse, error) {
	response := &types.UserGetTagListResponse{}
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
		return response, nil
	}
	response.Total = int(t.redis.ZCard(ctx, key).Val())

	return response, nil
}

// UserGetArticleListByTag 获取标签下的文章列表
func (t *tagService) UserGetArticleListByTag(ctx context.Context,
	request *types.UserGetArticleListByTagRequest) (*types.UserGetArticleListByTagResponse, error) {

	// 计算偏移量
	start := (request.Page - 1) * request.PageSize
	stop := start + request.PageSize - 1
	key := request.TagName + ":article:ZSet"
	response := &types.UserGetArticleListByTagResponse{}

	// 获取文章ID列表(包含分页条件)
	articleIDList, err := t.redis.ZRevRange(ctx, key, int64(start), int64(stop)).Result()
	if err != nil {
		t.logger.Error("failed to get article:ZSet", zap.Error(err))
		return nil, fmt.Errorf("failed to get article:ZSet: %w", err)
	}
	// 如果Redis中没有这个有序集合
	if len(articleIDList) == 0 {
		articleList, err := t.tagModel.GetArticleListByTagName(ctx, request.TagName)
		if err != nil {
			t.logger.Error("failed to get tagIDArticleZSet", zap.Error(err))
			return nil, err
		}
		if len(articleList) == 0 {
			t.logger.Error("not found tagName", zap.Error(err))
			return nil, fmt.Errorf("not found tagName")
		}
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
	fields := []string{"title", "describe", "viewNum", "createTime"}
	for _, articleID := range articleIDList {
		articleItem := types.UserGetArticleItem{}

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
			articleItem.ID = articleID
			articleItem.Title = articleInfo.Title
			articleItem.ViewNum = int(articleInfo.ViewNum)
			articleItem.CreateTime = articleInfo.CreateTime.Format(constants.TimeLayoutToMinute)
			articleItem.UpdateTime = articleInfo.UpdateTime.Format(constants.TimeLayoutToMinute)

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
			if err = t.redis.HMSet(ctx, "article:"+articleItem.ID+":Hash", mapData).Err(); err != nil {
				t.logger.Error("failed to write article:articleID:ZSet", zap.Error(err))
				return nil, fmt.Errorf("failed to write article:articleID:ZSet: %w", err)
			}
		}

		data, err := t.redis.HMGet(ctx, "article:"+articleID+":Hash", fields...).Result()
		if err != nil {
			t.logger.Error("failed to get article:ZSet", zap.Error(err))
			return nil, err
		}
		viewNumStr := data[1].(string)
		articleItem.ViewNum, err = strconv.Atoi(viewNumStr)
		if err != nil {
			t.logger.Error("parse string to int error", zap.Error(err))
			return nil, fmt.Errorf("parse string to int error, err: %w", err)
		}
		articleItem = types.UserGetArticleItem{
			ID:         articleID,
			Title:      data[0].(string),
			Describe:   data[1].(string),
			CreateTime: data[3].(string)[:10],
		}
		response.Rows = append(response.Rows, articleItem)
	}

	response.Total = int(t.redis.ZCard(ctx, key).Val())
	return response, nil
}

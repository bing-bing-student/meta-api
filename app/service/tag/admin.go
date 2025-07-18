package tag

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"meta-api/app/model/article"
	"meta-api/app/model/tag"
	"meta-api/common/constants"
	"meta-api/common/types"
)

// AdminGetTagList 获取标签列表
func (t *tagService) AdminGetTagList(ctx context.Context) (*types.AdminGetTagListResponse, error) {
	response := &types.AdminGetTagListResponse{}
	key := "tag:articleNum:ZSet"

	if exist := t.redis.Exists(ctx, key).Val(); exist == 0 {
		articleCountWithTagNameList, err := t.model.GetArticleCountWithTagName(ctx)
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

// AdminGetArticleListByTag 通过标签获取文章列表
func (t *tagService) AdminGetArticleListByTag(ctx context.Context,
	request *types.AdminGetArticleListByTagRequest) (*types.AdminGetArticleListByTagResponse, error) {

	response := &types.AdminGetArticleListByTagResponse{}
	start := (request.Page - 1) * request.PageSize
	stop := start + request.PageSize - 1
	key := request.TagName + ":article:ZSet"

	// 获取文章ID列表(包含分页条件)
	articleIDList, err := t.redis.ZRevRange(ctx, key, int64(start), int64(stop)).Result()
	if err != nil {
		t.logger.Error("failed to get article:ZSet", zap.Error(err))
		return nil, err
	}
	// 如果Redis中没有这个有序集合
	if len(articleIDList) == 0 {
		tagIDArticleZSet, err := t.model.GetTagNameArticleZSetByTagName(req.TagName)
		if err != nil {
			global.Logger.Error("failed to get tagIDArticleZSet", zap.Error(err))
			return err
		}
		for _, label := range tagIDArticleZSet {
			if err = t.redis.ZAdd(ctx, key, redis.Z{
				Score:  float64(label.CreateTime.UnixNano() / int64(time.Millisecond)),
				Member: label.ID,
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

		exists, err := global.RedisSentinel.Exists(global.Context, "article:"+articleID+":Hash").Result()
		if err != nil {
			global.Logger.Error("failed to get article:ZSet", zap.Error(err))
			return err
		}
		if exists == 0 {
			id, err := strconv.Atoi(articleID)
			if err != nil {
				global.Logger.Error("parse int error", zap.Error(err))
				return err
			}
			articleModel, mysqlErr := article.GetArticleDetailByID(uint64(id))
			if mysqlErr != nil {
				global.Logger.Error("failed to get article from MySQL", zap.Error(mysqlErr))
				return mysqlErr
			}
			articleItem.ID = articleID
			articleItem.Title = articleModel.Title
			articleItem.ViewNum = int(articleModel.ViewNum)
			articleItem.CreateTime = articleModel.CreateTime.Format(constants.TimeLayoutToMinute)
			articleItem.UpdateTime = articleModel.UpdateTime.Format(constants.TimeLayoutToMinute)

			mapData := map[string]interface{}{
				"id":         articleModel.ID,
				"title":      articleModel.Title,
				"describe":   articleModel.Describe,
				"content":    articleModel.Content,
				"viewNum":    articleModel.ViewNum,
				"createTime": articleModel.CreateTime.Format(global.TimeLayoutToSecond),
				"updateTime": articleModel.UpdateTime.Format(global.TimeLayoutToSecond),
				"tagID":      articleModel.TagID,
				"tagName":    articleModel.TagName,
			}
			if err = global.RedisSentinel.HMSet(global.Context, "article:"+articleItem.ID+":Hash", mapData).Err(); err != nil {
				global.Logger.Error("failed to write article:articleID:ZSet", zap.Error(err))
				return err
			}
		}
		data, err := global.RedisSentinel.HMGet(global.Context, "article:"+articleID+":Hash", fields...).Result()
		if err != nil {
			global.Logger.Error("failed to get article:ZSet", zap.Error(err))
			return err
		}
		articleItem.ID = articleID
		articleItem.Title = data[0].(string)
		viewNumStr := data[1].(string)
		articleItem.ViewNum, _ = strconv.Atoi(viewNumStr)
		articleItem.CreateTime = data[2].(string)
		articleItem.UpdateTime = data[3].(string)
		resp.Rows = append(resp.Rows, articleItem)
	}
	resp.Total = int(global.RedisSentinel.ZCard(global.Context, key).Val())
	return nil
}

// AdminUpdateTag 更新标签
func (t *tagService) AdminUpdateTag(ctx context.Context, request *types.AdminUpdateTagRequest) error {

	// 查找数据库中是否已存在相同名称的标签
	tagModel := new(tag.Tag)
	if err = tx.Model(&tag.Tag{}).Where("name = ?", req.NewTagName).First(tagModel).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// 如果标签不存在，则需要插入新标签
			tagID, err := global.IDGenerator.NextID()
			if err != nil {
				global.Logger.Error("generate id error", zap.Error(err))
				return errors.New("generate id error")
			}
			tagModel.ID = tagID
			tagModel.Name = req.NewTagName
			if err = tx.Create(&tagModel).Error; err != nil {
				global.Logger.Error("failed to create new tag", zap.Error(err))
				return err
			}

			// 更新文章表中的标签ID
			if err = tx.Model(&article.Article{}).Where("id IN ?", req.ArticleIDList).
				Update("tag_id", tagID).Error; err != nil {
				global.Logger.Error("failed to update article tags", zap.Error(err))
				return err
			}
		} else {
			global.Logger.Error("failed to query tag", zap.Error(err))
			return err
		}
	}

	// 如果前端提供的tag在MySQL当中存在,那么需要更新文章表当中的tag_id
	if err = tx.Model(&article.Article{}).
		Where("id IN ?", req.ArticleIDList).
		Update("tag_id", tx.Model(&tag.Tag{}).Select("id").Where("name = ?", req.NewTagName)).Error; err != nil {
		global.Logger.Error("failed to update tag id for article", zap.Error(err))
		return err
	}

	// 更新标签之前先将缓存当中的浏览量数据写入mysql
	for _, articleID := range req.ArticleIDList {
		viewNum, err := global.RedisSentinel.ZScore(global.Context, "article:view:ZSet", articleID).Result()
		if err != nil {
			global.Logger.Error("failed to query article:view:ZSet", zap.Error(err))
			tx.Rollback()
			return err
		}
		if err = tx.Model(&article.Article{}).Where("id = ?", articleID).Update("view_num", viewNum).Error; err != nil {
			global.Logger.Error("failed to update article view num", zap.Error(err))
			tx.Rollback()
			return err
		}
	}
	if err = tx.Commit().Error; err != nil {
		global.Logger.Error("failed to commit transaction", zap.Error(err))
		return err
	}

	// Redis更新数据
	for _, item := range req.ArticleIDList {
		if err = global.RedisSentinel.Del(global.Context, "article:"+item+":Hash").Err(); err != nil {
			global.Logger.Error("failed to delete article:Hash", zap.Error(err))
			return err
		}
	}

	if err = global.RedisSentinel.Del(global.Context, req.OldTagName+":article:ZSet").Err(); err != nil {
		global.Logger.Error("failed to delete tag:article:ZSet", zap.Error(err))
		return err
	}

	if err = global.RedisSentinel.Del(global.Context, req.NewTagName+":article:ZSet").Err(); err != nil {
		global.Logger.Error("failed to delete tag:article:ZSet", zap.Error(err))
		return err
	}

	if err = global.RedisSentinel.Del(global.Context, "tag:articleNum:ZSet").Err(); err != nil {
		global.Logger.Error("failed to delete tag:articleNum:ZSet", zap.Error(err))
		return err
	}
}

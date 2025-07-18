package tag

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"meta-api/app/model/article"
	"meta-api/common/types"
)

// UserGetTagList 获取标签列表
func (t *tagService) UserGetTagList(ctx context.Context) (types.UserGetTagListResponse, error) {
	exist, err := global.RedisSentinel.Exists(global.Context, "tag:articleNum:ZSet").Result()
	if err != nil {
		global.Logger.Error("failed to get tag:articleNum:ZSet", zap.Error(err))
		return err
	}
	if exist == 1 {
		tagZSet, err := global.RedisSentinel.ZRevRangeWithScores(global.Context, "tag:articleNum:ZSet", 0, -1).Result()
		if err != nil {
			global.Logger.Error("failed to get tag:articleNum:ZSet", zap.Error(err))
			return err
		}

		tagListItem := make([]types.UserGetTagListItem, 0)
		for _, label := range tagZSet {
			tagListItem = append(tagListItem, types.UserGetTagListItem{
				Name:       label.Member.(string),
				ArticleNum: int(label.Score),
			})
		}
		resp.Rows = tagListItem
		resp.Total = len(tagZSet)
		return nil
	}
	if exist == 0 {
		tagList := make([]article.TagWithArticleCount, 0)
		if err = global.MySqlDB.
			Table("tag").
			Select("tag.name, COUNT(article.id) AS count").
			Joins("JOIN article ON article.tag_id = tag.id").
			Group("tag.id").
			Having("COUNT(article.id) > 0").
			Find(&tagList).
			Error; err != nil {
			global.Logger.Error("failed to get tags with article", zap.Error(err))
			return err
		}

		zAddArgs := make([]redis.Z, len(tagList))
		for i, data := range tagList {
			zAddArgs[i] = redis.Z{
				Score:  float64(data.Count),
				Member: data.Name,
			}
			resp.Rows = append(resp.Rows, types.UserGetTagListItem{
				Name:       data.Name,
				ArticleNum: data.Count,
			})
		}
		resp.Total = len(tagList)

		// 批量写入Redis
		if err = global.RedisSentinel.ZAdd(global.Context, "tag:articleNum:ZSet", zAddArgs...).Err(); err != nil {
			global.Logger.Error("failed to write tag:articleNum:ZSet", zap.Error(err))
			return err
		}
	}
}

// UserGetArticleListByTag 获取标签下的文章列表
func (t *tagService) UserGetArticleListByTag(ctx context.Context,
	request *types.UserGetArticleListByTagRequest) (types.UserGetArticleListByTagResponse, error) {

	// 计算偏移量
	start := (req.Page - 1) * req.PageSize
	stop := start + req.PageSize - 1

	key := req.TagName + ":article:ZSet"
	// 获取文章ID列表(包含分页条件)

	articleIDList, err := global.RedisSentinel.ZRevRange(global.Context, key, int64(start), int64(stop)).Result()
	if err != nil {
		global.Logger.Error("failed to get article:ZSet", zap.Error(err))
		return err
	}
	// 如果Redis中没有这个有序集合
	if len(articleIDList) == 0 {
		tagIDArticleZSet, err := article.GetTagNameArticleZSetByTagName(req.TagName)
		if err != nil {
			global.Logger.Error("failed to get tagIDArticleZSet", zap.Error(err))
			return errors.New("service internal error")
		}
		if len(tagIDArticleZSet) == 0 {
			global.Logger.Error("not found tagName", zap.Error(err))
			return errors.New("not found tagName")
		}
		for _, label := range tagIDArticleZSet {
			if err = global.RedisSentinel.ZAdd(global.Context, key, redis.Z{
				Score:  float64(label.CreateTime.UnixNano() / int64(time.Millisecond)),
				Member: label.ID,
			}).Err(); err != nil {
				global.Logger.Error("failed to write article:ZSet", zap.Error(err))
				return err
			}
		}
		// 再次获取数据(包含分页条件)
		articleIDList, err = global.RedisSentinel.ZRevRange(global.Context, key, int64(start), int64(stop)).Result()
		if err != nil {
			global.Logger.Error("failed to get article:ZSet", zap.Error(err))
			return err
		}
	}
	// 获取Redis当中的文章Hash数据
	fields := []string{"title", "describe", "viewNum", "createTime"}
	for _, articleID := range articleIDList {
		articleItem := types.UserGetArticleListItem{}
		if exists := global.RedisSentinel.Exists(global.Context, "article:"+articleID+":Hash").Val(); exists == 0 {
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
			articleItem.CreateTime = articleModel.CreateTime.Format(global.TimeLayoutToMinute)
			articleItem.UpdateTime = articleModel.UpdateTime.Format(global.TimeLayoutToMinute)

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
		if err != nil && errors.Is(err, redis.Nil) {
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
			articleItem.Describe = articleModel.Describe
			articleItem.ViewNum = int(articleModel.ViewNum)
			articleItem.CreateTime = articleModel.CreateTime.Format(global.TimeLayoutToMinute)

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
			global.RedisSentinel.HMSet(global.Context, "article:"+articleItem.ID+":Hash", mapData)
			resp.Rows = append(resp.Rows, articleItem)
			continue
		} else if err != nil {
			global.Logger.Error("failed to get article:ZSet", zap.Error(err))
			return err
		}
		viewNum, err := strconv.Atoi(data[2].(string))
		if err != nil {
			global.Logger.Error("parse int error", zap.Error(err))
			return err
		}
		articleItem = types.UserGetArticleListItem{
			ID:         articleID,
			Title:      data[0].(string),
			Describe:   data[1].(string),
			ViewNum:    viewNum,
			CreateTime: data[3].(string)[:10],
		}
		resp.Rows = append(resp.Rows, articleItem)
	}

	totalIDList, err := global.RedisSentinel.ZRevRange(global.Context, key, 0, -1).Result()
	if err != nil {
		global.Logger.Error("failed to get article:ZSet", zap.Error(err))
		return err
	}
	resp.Total = len(totalIDList)
}

package tag

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"meta-api/app/model/article"
	"meta-api/app/model/tag"
	"meta-api/common/types"
)

// AdminGetTagList 获取标签列表
func (t *tagService) AdminGetTagList(ctx context.Context) (types.AdminGetTagListResponse, error) {
	exist, err := global.RedisSentinel.Exists(global.Context, "tag:articleNum:ZSet").Result()
	if err != nil {
		global.Logger.Error("failed to get tag:articleNum:ZSet", zap.Error(err))
		return nil, err
	}
	if exist == 1 {
		tagZSet, err := global.RedisSentinel.ZRevRangeWithScores(global.Context, "tag:articleNum:ZSet", 0, -1).Result()
		if err != nil {
			global.Logger.Error("failed to get tag:articleNum:ZSet", zap.Error(err))
			return nil, err
		}

		for _, label := range tagZSet {
			resp = append(resp, label.Member.(string))
		}
		return resp, nil
	}
	if exist == 0 {
		tagList := make([]article.TagWithArticleCount, 0)
		if err = global.MySqlDB.
			Table("tag").
			Select("tag.name, COUNT(article.id) AS count").
			Joins("JOIN article ON article.tag_id = tag.id").
			Group("tag.id").
			Having("COUNT(article.id) > 0").
			Order("count DESC").
			Find(&tagList).
			Error; err != nil {
			global.Logger.Error("failed to get tags with article", zap.Error(err))
			return nil, err
		}

		if len(tagList) > 0 {
			zAddArgs := make([]redis.Z, len(tagList))
			for i, data := range tagList {
				zAddArgs[i] = redis.Z{
					Score:  float64(data.Count),
					Member: data.Name,
				}
				resp = append(resp, data.Name)
			}

			// 批量写入Redis
			if err = global.RedisSentinel.ZAdd(global.Context, "tag:articleNum:ZSet", zAddArgs...).Err(); err != nil {
				global.Logger.Error("failed to write tag:articleNum:ZSet", zap.Error(err))
				return nil, err
			}
		}
	}
	return resp, nil
}

// AdminGetArticleListByTag 通过标签获取文章列表
func (t *tagService) AdminGetArticleListByTag(ctx context.Context,
	request *types.AdminGetArticleListByTagRequest) (types.AdminGetArticleListByTagResponse, error) {

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
			return err
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
func (t *tagService) AdminUpdateTag(ctx context.Context, request types.AdminUpdateTagRequest) error {
	// MySQL更新数据
	tx := global.MySqlDB.Begin()
	if tx.Error != nil {
		global.Logger.Error("failed to start transaction", zap.Error(tx.Error))
		return tx.Error
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

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

package article

import (
	"context"
	"strconv"

	"go.uber.org/zap"

	"meta-api/app/model/article"
	"meta-api/common/constants"
	"meta-api/common/types"
)

// AdminGetArticleList 管理员获取文章列表
func (a *articleService) AdminGetArticleList(ctx context.Context,
	req *types.AdminGetArticleListRequest, resp *types.AdminGetArticleListResponse) (err error) {

	// 计算偏移量
	start := (req.Page - 1) * req.PageSize
	stop := start + req.PageSize - 1

	zSetKey := "article:" + req.Order + ":ZSet"
	// 获取文章ID有序集合
	articleIDZSet, err := a.redis.ZRevRangeWithScores(ctx, zSetKey, int64(start), int64(stop)).Result()
	if err != nil {
		a.logger.Error("failed to get article:time/view:ZSet", zap.Error(err))
		return err
	}
	articleList := make([]types.AdminGetArticleListItem, 0)
	for _, z := range articleIDZSet {
		articleItem := types.AdminGetArticleListItem{}
		articleItem.ID = z.Member.(string)
		// 获取数据
		hashKey := "article:" + z.Member.(string) + ":Hash"
		if exist := a.redis.Exists(ctx, hashKey); exist.Val() == 1 {
			// redis当中存在该数据
			fields := []string{"title", "tagName", "viewNum", "createTime", "updateTime"}
			result, err := a.redis.HMGet(ctx, hashKey, fields...).Result()
			if err != nil {
				return err
			}
			articleItem.Title = result[0].(string)
			articleItem.Tag = result[1].(string)
			viewNumStr := result[2].(string)
			articleItem.ViewNum, _ = strconv.Atoi(viewNumStr)
			articleItem.CreateTime = result[3].(string)[:16]
			articleItem.UpdateTime = result[4].(string)[:16]
		} else {
			// redis当中不存在该数据
			articleModel := new(article.Detail)
			id, err := strconv.ParseUint(z.Member.(string), 10, 64)
			if err != nil {
				a.logger.Error("parse uint64 error", zap.Error(err))
				return err
			}
			if articleModel, err = a.model.GetArticleDetailByID(id); err != nil {
				a.logger.Error("get article detail by id error", zap.Error(err))
				return err
			}
			articleItem.Title = articleModel.Title
			articleItem.Tag = articleModel.TagName
			articleItem.ViewNum = int(articleModel.ViewNum)
			articleItem.CreateTime = articleModel.CreateTime.Format(constants.TimeLayoutToMinute)
			articleItem.UpdateTime = articleModel.UpdateTime.Format(constants.TimeLayoutToMinute)

			mapData := map[string]interface{}{
				"id":         articleModel.ID,
				"title":      articleModel.Title,
				"describe":   articleModel.Describe,
				"content":    articleModel.Content,
				"viewNum":    articleModel.ViewNum,
				"createTime": articleModel.CreateTime.Format(constants.TimeLayoutToSecond),
				"updateTime": articleModel.UpdateTime.Format(constants.TimeLayoutToSecond),
				"tagID":      articleModel.TagID,
				"tagName":    articleModel.TagName,
			}
			a.redis.HMSet(ctx, "article:"+articleItem.ID+":Hash", mapData)
		}
		articleList = append(articleList, articleItem)
	}

	resp.Rows = articleList
	resp.Total = int(a.redis.ZCard(ctx, zSetKey).Val())
	return nil
}

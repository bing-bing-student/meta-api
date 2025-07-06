package article

import (
	"strconv"

	"go.uber.org/zap"

	"meta-api/internal/common/types"
)

// AdminGetArticleListService 管理员获取文章列表
func AdminGetArticleListService(req *types.AdminGetArticleListRequest, resp *types.AdminGetArticleListResponse) (err error) {
	// 计算偏移量
	start := (req.Page - 1) * req.PageSize
	stop := start + req.PageSize - 1

	zSetKey := ""
	switch req.Order {
	case "time":
		zSetKey = "article:time:ZSet"
	case "view":
		zSetKey = "article:view:ZSet"
	}
	// 获取文章ID有序集合
	articleIDZSet, err := global.RedisSentinel.ZRevRangeWithScores(global.Context, zSetKey, int64(start), int64(stop)).Result()
	if err != nil {
		global.Logger.Error("failed to get article:time/view:ZSet", zap.Error(err))
		return err
	}
	articleList := make([]types.AdminGetArticleListItem, 0)
	for _, z := range articleIDZSet {
		articleItem := types.AdminGetArticleListItem{}
		articleItem.ID = z.Member.(string)
		// 获取数据
		hashKey := "article:" + z.Member.(string) + ":Hash"
		if exist := global.RedisSentinel.Exists(global.Context, hashKey); exist.Val() == 1 {
			// redis当中存在该数据
			fields := []string{"title", "tagName", "viewNum", "createTime", "updateTime"}
			result, err := global.RedisSentinel.HMGet(global.Context, hashKey, fields...).Result()
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
				global.Logger.Error("parse uint64 error", zap.Error(err))
				return err
			}
			if articleModel, err = article.GetArticleDetailByID(id); err != nil {
				global.Logger.Error("get article detail by id error", zap.Error(err))
				return err
			}
			articleItem.Title = articleModel.Title
			articleItem.Tag = articleModel.TagName
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
			global.RedisSentinel.HMSet(global.Context, "article:"+articleItem.ID+":Hash", mapData)
		}
		articleList = append(articleList, articleItem)
	}

	resp.Rows = articleList
	resp.Total = int(global.RedisSentinel.ZCard(global.Context, zSetKey).Val())
	return nil
}

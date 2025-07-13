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
	request *types.AdminGetArticleListRequest) (*types.AdminGetArticleListResponse, error) {

	response := &types.AdminGetArticleListResponse{}
	// 计算偏移量
	start := (request.Page - 1) * request.PageSize
	stop := start + request.PageSize - 1

	zSetKey := "article:" + request.Order + ":ZSet"
	// 获取文章ID有序集合
	articleIDZSet, err := a.redis.ZRevRangeWithScores(ctx, zSetKey, int64(start), int64(stop)).Result()
	if err != nil {
		a.logger.Error("failed to get article:time/view:ZSet", zap.Error(err))
		return response, err
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
				return response, err
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
				return response, err
			}
			if articleModel, err = a.model.GetArticleDetailByID(id); err != nil {
				a.logger.Error("get article detail by id error", zap.Error(err))
				return response, err
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

	response.Rows = articleList
	response.Total = int(a.redis.ZCard(ctx, zSetKey).Val())

	return response, nil
}

// AdminGetArticleDetail 获取文章详情
func (a *articleService) AdminGetArticleDetail(ctx context.Context,
	request *types.AdminGetArticleDetailRequest) (*types.AdminGetArticleDetailResponse, error) {

	hashKey := "article:" + req.ID + ":Hash"
	if exist := global.RedisSentinel.Exists(global.Context, hashKey); exist.Val() == 1 {
		// redis当中存在该数据
		fields := []string{"id", "title", "tagName", "describe", "content"}
		result, err := global.RedisSentinel.HMGet(global.Context, hashKey, fields...).Result()
		if err != nil {
			global.Logger.Error("hmget error", zap.Error(err))
			return err
		}
		resp.ID = result[0].(string)
		resp.Title = result[1].(string)
		resp.Tag = result[2].(string)
		resp.Describe = result[3].(string)
		resp.Content = result[4].(string)
	} else {
		// redis当中不存在该数据
		articleModel := &article.Detail{}
		id, err := strconv.ParseUint(req.ID, 10, 64)
		if err != nil {
			global.Logger.Error("parse uint64 error", zap.Error(err))
			return err
		}
		if articleModel, err = article.GetArticleDetailByID(id); err != nil {
			global.Logger.Error("get article detail by id error", zap.Error(err))
			return err
		}
		resp.ID = strconv.FormatUint(articleModel.ID, 10)
		resp.Title = articleModel.Title
		resp.Tag = articleModel.TagName
		resp.Describe = articleModel.Describe
		resp.Content = articleModel.Content

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
		global.RedisSentinel.HMSet(global.Context, "article:"+resp.ID+":Hash", mapData)
	}

	return response, nil
}

// AdminAddArticle 添加文章
func (a *articleService) AdminAddArticle(ctx context.Context,
	request *types.AdminAddArticleRequest) error {

	return nil
}

// AdminUpdateArticle 更新文章
func (a *articleService) AdminUpdateArticle(ctx context.Context,
	request *types.AdminUpdateArticleRequest) error {

	return nil
}

// AdminDeleteArticle 删除文章
func (a *articleService) AdminDeleteArticle(ctx context.Context,
	request *types.AdminDeleteArticleRequest) error {

	return nil
}

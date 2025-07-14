package article

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"meta-api/app/model/article"
	"meta-api/common/types"
)

// UserGetArticleList 获取文章列表
func (a *articleService) UserGetArticleList(ctx context.Context,
	request *types.UserGetArticleListRequest) (*types.UserGetArticleListResponse, error) {

	response := &types.UserGetArticleListResponse{}

	// 计算偏移量
	start := (req.Page - 1) * req.PageSize
	stop := start + req.PageSize - 1

	// 获取文章ID有序集合
	articleIDZSet, err := a.redis.ZRevRangeWithScores(ctx, "article:time:ZSet",
		int64(start), int64(stop)).Result()
	if err != nil {
		a.logger.Error("failed to get article:time:ZSet", zap.Error(err))
		return err
	}
	articleList := make([]types.UserGetArticleListItem, 0)
	for _, z := range articleIDZSet {
		articleItem := types.UserGetArticleListItem{}
		articleItem.ID = z.Member.(string)
		hashKey := "article:" + z.Member.(string) + ":Hash"
		if exist := a.redis.Exists(ctx, hashKey); exist.Val() == 1 {
			fields := []string{"title", "tagName", "describe", "createTime", "updateTime", "viewNum"}
			result, err := a.redis.HMGet(ctx, hashKey, fields...).Result()
			if err != nil {
				return err
			}
			articleItem.Title = result[0].(string)
			articleItem.TagName = result[1].(string)
			articleItem.Describe = result[2].(string)
			articleItem.CreateTime = result[3].(string)[:10]
			articleItem.UpdateTime = result[4].(string)[:10]
			viewNumStr := result[5].(string)
			articleItem.ViewNum, _ = strconv.Atoi(viewNumStr)
		} else {
			articleModel := new(article.Detail)
			id, err := strconv.ParseUint(z.Member.(string), 10, 64)
			if err != nil {
				a.logger.Error("parse uint64 error", zap.Error(err))
				return err
			}
			if articleModel, err = article.GetArticleDetailByID(id); err != nil {
				a.logger.Error("get article detail by id error", zap.Error(err))
				return err
			}
			articleItem.Title = articleModel.Title
			articleItem.TagName = articleModel.TagName
			articleItem.Describe = articleModel.Describe
			articleItem.CreateTime = articleModel.CreateTime.Format(global.TimeLayoutToDay)
			articleItem.UpdateTime = articleModel.UpdateTime.Format(global.TimeLayoutToDay)
			articleItem.ViewNum = int(articleModel.ViewNum)

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
			a.redis.HMSet(ctx, "article:"+articleItem.ID+":Hash", mapData)
		}
		articleList = append(articleList, articleItem)
	}

	resp.Rows = articleList
	resp.Total = int(a.redis.ZCard(ctx, "article:time:ZSet").Val())

	return response, nil
}

// UserGetArticleDetail 获取文章详情
func (a *articleService) UserGetArticleDetail(ctx context.Context,
	request *types.UserGetArticleDetailRequest) (*types.UserGetArticleDetailResponse, error) {

	response := &types.UserGetArticleDetailResponse{}
	hashKey := "article:" + req.ID + ":Hash"
	if exist := a.redis.Exists(ctx, hashKey); exist.Val() == 1 {
		// redis当中存在该数据
		fields := []string{"title", "tagName", "content", "createTime", "updateTime"}
		result, err := a.redis.HMGet(ctx, hashKey, fields...).Result()
		if err != nil {
			a.logger.Error("hmget error", zap.Error(err))
			return err
		}
		resp.ID = req.ID
		resp.Title = result[0].(string)
		resp.TagName = result[1].(string)
		resp.Content = result[2].(string)
		resp.CreateTime = result[3].(string)[:10]
		resp.UpdateTime = result[4].(string)[:10]
	} else {
		// redis当中不存在该数据
		articleModel := &article.Detail{}
		id, err := strconv.ParseUint(req.ID, 10, 64)
		if err != nil {
			a.logger.Error("parse uint64 error", zap.Error(err))
			return err
		}
		if articleModel, err = article.GetArticleDetailByID(id); err != nil {
			a.logger.Error("get article detail by id error", zap.Error(err))
			return errors.New("failed to get article detail by id")
		}
		resp.ID = strconv.FormatUint(articleModel.ID, 10)
		resp.Title = articleModel.Title
		resp.TagName = articleModel.TagName
		resp.Content = articleModel.Content
		resp.CreateTime = articleModel.CreateTime.Format(global.TimeLayoutToMinute)
		resp.UpdateTime = articleModel.UpdateTime.Format(global.TimeLayoutToMinute)

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
		a.redis.HMSet(ctx, "article:"+resp.ID+":Hash", mapData)
	}

	// 浏览量自增
	lua := redis.NewScript(`
	local articleID = KEYS[1]
	local userID = KEYS[2]
	local expireTime = tonumber(ARGV[1])
	
	local isSet = redis.call('SETNX', articleID .. ':' .. userID, 1)
	if isSet == 1 then
		redis.call('EXPIRE', articleID .. ':' .. userID, expireTime)
		redis.call('HINCRBY', 'article:' .. articleID .. ':Hash', 'viewNum', 1)
		redis.call('ZINCRBY', 'article:view:ZSet', 1, articleID)
	end
	
	return isSet`)

	expireTime := 30
	result, err := lua.Run(ctx, a.redis, []string{req.ID, userID}, expireTime).Int()
	if err != nil {
		a.logger.Error("failed to run lua script", zap.Int("result of value: ", result))
		return err
	}

	return response, nil
}

// UserSearchArticle 搜索文章
func (a *articleService) UserSearchArticle(ctx context.Context,
	request *types.UserSearchArticleRequest) (*types.UserSearchArticleResponse, error) {

	response := &types.UserSearchArticleResponse{}

	//将输入进行转义（防止XSS攻击）
	word := html.EscapeString(req.Word)

	// 在mysql里面进行模糊查询
	pageSize := 9
	offset := (req.Page - 1) * pageSize

	// 使用 GORM 进行查询，忽略大小写
	var total int64
	articleList := make([]article.Article, 0)
	if err = global.MySqlDB.Model(&article.Article{}).
		Where("LOWER(title) LIKE LOWER(?)", fmt.Sprintf("%%%s%%", word)).
		Count(&total).
		Limit(pageSize).Offset(offset).
		Find(&articleList).Error; err != nil {
		return fmt.Errorf("failed to query articles: %v", err)
	}

	items := make([]types.UserGetArticleListItem, 0)
	for _, item := range articleList {
		id := fmt.Sprintf("%d", item.ID)
		viewNum, err := a.redis.ZScore(ctx, "article:view:ZSet", id).Result()
		if err != nil {
			a.logger.Error("failed to query article:view:ZSet", zap.Error(err))
			return err
		}
		items = append(items, types.UserGetArticleListItem{
			ID:         id,
			Title:      item.Title,
			TagName:    item.Tag.Name,
			Describe:   item.Describe,
			CreateTime: item.CreateTime.Format(global.TimeLayoutToDay),
			UpdateTime: item.UpdateTime.Format(global.TimeLayoutToDay),
			ViewNum:    int(viewNum),
		})
	}

	resp.Rows = items
	resp.Total = int(total)

	return response, nil
}

// UserGetHotArticle 获取热门文章
func (a *articleService) UserGetHotArticle(ctx context.Context) (*types.UserGetHotArticleResponse, error) {
	response := &types.UserGetHotArticleResponse{}

	// 获取文章ID有序集合
	articleIDZSet, err := a.redis.ZRevRangeWithScores(ctx, "article:view:ZSet", 0, 2).Result()
	if err != nil {
		a.logger.Error("failed to get article:view:ZSet", zap.Error(err))
		return err
	}

	articleList := make([]types.GetHotArticleItem, 0)
	for _, z := range articleIDZSet {
		articleItem := types.GetHotArticleItem{}
		articleItem.ID = z.Member.(string)

		hashKey := "article:" + z.Member.(string) + ":Hash"
		if exist := a.redis.Exists(ctx, hashKey); exist.Val() == 1 {
			fields := []string{"title", "viewNum"}
			result, err := a.redis.HMGet(ctx, hashKey, fields...).Result()
			if err != nil {
				a.logger.Error("failed to get hash data", zap.Error(err))
				return err
			}
			articleItem.Title = result[0].(string)
			if articleItem.ViewNum, err = strconv.Atoi(result[1].(string)); err != nil {
				a.logger.Error("parse int error", zap.Error(err))
				return err
			}
		} else {
			articleModel := new(article.Detail)
			id, err := strconv.ParseUint(z.Member.(string), 10, 64)
			if err != nil {
				a.logger.Error("parse uint64 error", zap.Error(err))
				return err
			}
			if articleModel, err = article.GetArticleDetailByID(id); err != nil {
				a.logger.Error("get article detail by id error", zap.Error(err))
				return err
			}
			articleItem.Title = articleModel.Title
			articleItem.ViewNum = int(articleModel.ViewNum)
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
			a.redis.HMSet(ctx, "article:"+articleItem.ID+":Hash", mapData)
		}
		articleList = append(articleList, articleItem)
	}
	resp.Rows = articleList
	resp.Total = len(articleList)

	return response, nil
}

// UserGetTimeline 获取文章归档
func (a *articleService) UserGetTimeline(ctx context.Context) (*types.GetTimelineResponse, error) {
	response := &types.GetTimelineResponse{}

	articleIDZSet, err := a.redis.ZRevRangeWithScores(ctx, "article:time:ZSet", 0, -1).Result()
	if err != nil {
		a.logger.Error("failed to get article:time:ZSet", zap.Error(err))
		return err
	}
	groupedArticles := make(map[string][]types.GetTimelineListItem)
	for _, z := range articleIDZSet {
		hashKey := "article:" + z.Member.(string) + ":Hash"
		if exist := a.redis.Exists(ctx, hashKey); exist.Val() == 1 {
			result, err := a.redis.HMGet(ctx, hashKey, []string{"title", "createTime"}...).Result()
			if err != nil {
				a.logger.Error("failed to get hash data", zap.Error(err))
				return err
			}
			articleItem := types.GetTimelineListItem{
				ID:         z.Member.(string),
				Title:      result[0].(string),
				CreateTime: result[1].(string)[:16],
			}
			year := articleItem.CreateTime[:4]
			groupedArticles[year] = append(groupedArticles[year], articleItem)
		} else {
			articleModel := new(article.Detail)
			id, err := strconv.ParseUint(z.Member.(string), 10, 64)
			if err != nil {
				a.logger.Error("parse uint64 error", zap.Error(err))
				return err
			}
			if articleModel, err = article.GetArticleDetailByID(id); err != nil {
				a.logger.Error("get article detail by id error", zap.Error(err))
				return err
			}
			articleItem := types.GetTimelineListItem{
				ID:         strconv.Itoa(int(articleModel.ID)),
				Title:      articleModel.Title,
				CreateTime: articleModel.CreateTime.Format(global.TimeLayoutToMinute),
			}
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
			a.redis.HMSet(ctx, "article:"+articleItem.ID+":Hash", mapData)
			groupedArticles[articleItem.CreateTime[:4]] = append(groupedArticles[articleItem.CreateTime[:4]], articleItem)
		}
	}
	var rows []types.GetTimelineRowsItem
	years := make([]string, 0, len(groupedArticles))
	for year := range groupedArticles {
		years = append(years, year)
	}
	sort.Slice(years, func(i, j int) bool {
		return years[i] > years[j]
	})
	for _, year := range years {
		rows = append(rows, types.GetTimelineRowsItem{
			Time: year,
			List: groupedArticles[year],
		})
	}
	resp.Rows = rows
	resp.Total = len(articleIDZSet)

	return response, nil
}

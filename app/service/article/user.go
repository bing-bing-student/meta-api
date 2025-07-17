package article

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"meta-api/common/constants"
	"meta-api/common/types"
)

// UserGetArticleList 获取文章列表
func (a *articleService) UserGetArticleList(ctx context.Context,
	request *types.UserGetArticleListRequest) (*types.UserGetArticleListResponse, error) {

	response := &types.UserGetArticleListResponse{}
	start := (request.Page - 1) * request.PageSize
	stop := start + request.PageSize - 1

	// 获取文章ID有序集合
	articleIDZSet, err := a.redis.ZRevRangeWithScores(ctx, "article:time:ZSet", int64(start), int64(stop)).Result()
	if err != nil {
		a.logger.Error("failed to get article:time:ZSet", zap.Error(err))
		return nil, err
	}

	articleList := make([]types.UserGetArticleItem, 0)
	for _, z := range articleIDZSet {
		articleItem := types.UserGetArticleItem{}

		articleItem.ID = z.Member.(string)
		hashKey := "article:" + z.Member.(string) + ":Hash"
		if exist := a.redis.Exists(ctx, hashKey); exist.Val() == 1 {
			// 获取缓存数据
			fields := []string{"title", "tagName", "describe", "createTime", "updateTime", "viewNum"}
			result, err := a.redis.HMGet(ctx, hashKey, fields...).Result()
			if err != nil {
				a.logger.Error("get article info hmget error", zap.Error(err))
				return nil, fmt.Errorf("get article info hmget error, err: %w", err)
			}
			articleItem.Title = result[0].(string)
			articleItem.TagName = result[1].(string)
			articleItem.Describe = result[2].(string)
			articleItem.CreateTime = result[3].(string)[:10]
			articleItem.UpdateTime = result[4].(string)[:10]
			viewNumStr := result[5].(string)
			articleItem.ViewNum, err = strconv.Atoi(viewNumStr)
			if err != nil {
				a.logger.Error("parse string to int error", zap.Error(err))
				return nil, fmt.Errorf("parse string to int error, err: %w", err)
			}
		} else {
			// 查询数据库
			id, err := strconv.ParseUint(articleItem.ID, 10, 64)
			if err != nil {
				a.logger.Error("parse uint64 error", zap.Error(err))
				return nil, err
			}
			articleInfo, err := a.articleModel.GetArticleDetailByID(ctx, id)
			if err != nil {
				a.logger.Error("get article detail by id error", zap.Error(err))
				return nil, fmt.Errorf("get article detail by id error, err: %w", err)
			}

			// 设置缓存
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
			if err = a.redis.HMSet(ctx, "article:"+articleItem.ID+":Hash", mapData).Err(); err != nil {
				a.logger.Error("redis set article hash error", zap.Error(err))
				return nil, fmt.Errorf("redis set article hash error: %w", err)
			}

			// 返回数据
			articleItem.Title = articleInfo.Title
			articleItem.TagName = articleInfo.TagName
			articleItem.Describe = articleInfo.Describe
			articleItem.CreateTime = articleInfo.CreateTime.Format(constants.TimeLayoutToDay)
			articleItem.UpdateTime = articleInfo.UpdateTime.Format(constants.TimeLayoutToDay)
			articleItem.ViewNum = int(articleInfo.ViewNum)
		}
		articleList = append(articleList, articleItem)
	}
	response.Rows = articleList
	response.Total = int(a.redis.ZCard(ctx, "article:time:ZSet").Val())

	return response, nil
}

// UserGetArticleDetail 获取文章详情
func (a *articleService) UserGetArticleDetail(ctx context.Context,
	request *types.UserGetArticleDetailRequest) (*types.UserGetArticleDetailResponse, error) {

	response := &types.UserGetArticleDetailResponse{}
	hashKey := "article:" + request.ID + ":Hash"
	if exist := a.redis.Exists(ctx, hashKey); exist.Val() == 1 {
		// 缓存查询
		fields := []string{"title", "tagName", "content", "createTime", "updateTime"}
		result, err := a.redis.HMGet(ctx, hashKey, fields...).Result()
		if err != nil {
			a.logger.Error("get article info hmget error", zap.Error(err))
			return nil, err
		}
		response.ID = request.ID
		response.Title = result[0].(string)
		response.TagName = result[1].(string)
		response.Content = result[2].(string)
		response.CreateTime = result[3].(string)[:10]
		response.UpdateTime = result[4].(string)[:10]
	} else {
		// 查询MySQL
		id, err := strconv.ParseUint(request.ID, 10, 64)
		if err != nil {
			a.logger.Error("parse uint64 error", zap.Error(err))
			return nil, err
		}
		articleInfo, err := a.articleModel.GetArticleDetailByID(ctx, id)
		if err != nil {
			a.logger.Error("get article detail by id error", zap.Error(err))
			return nil, fmt.Errorf("get article detail by id error, err: %w", err)
		}

		// 设置缓存
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
		if err = a.redis.HMSet(ctx, "article:"+request.ID+":Hash", mapData).Err(); err != nil {
			a.logger.Error("redis set article hash error", zap.Error(err))
			return nil, fmt.Errorf("redis set article hash error: %w", err)
		}

		// 返回数据
		response.ID = request.ID
		response.Title = articleInfo.Title
		response.TagName = articleInfo.TagName
		response.Content = articleInfo.Content
		response.CreateTime = articleInfo.CreateTime.Format(constants.TimeLayoutToMinute)
		response.UpdateTime = articleInfo.UpdateTime.Format(constants.TimeLayoutToMinute)
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
	result, err := lua.Run(ctx, a.redis, []string{request.ID, request.UserID}, expireTime).Int()
	if err != nil {
		a.logger.Error("failed to run lua script", zap.Int("result of value: ", result))
		return nil, fmt.Errorf("failed to run lua script: %w", err)
	}

	return response, nil
}

// UserSearchArticle 搜索文章
func (a *articleService) UserSearchArticle(ctx context.Context,
	request *types.UserSearchArticleRequest) (*types.UserSearchArticleResponse, error) {

	response := &types.UserSearchArticleResponse{}
	limit := request.PageSize
	offset := (request.Page - 1) * request.PageSize
	word := strings.TrimSpace(request.Word)
	articleList, total, err := a.articleModel.SearchArticle(ctx, word, limit, offset)
	if err != nil {
		a.logger.Error("failed to search article", zap.Error(err))
		return nil, fmt.Errorf("failed to search article, err: %w", err)
	}

	rows := make([]types.UserGetArticleItem, 0)
	for _, item := range articleList {
		rows = append(rows, types.UserGetArticleItem{
			ID:       strconv.Itoa(int(item.ID)),
			Title:    item.Title,
			Describe: item.Describe,
			ViewNum:  int(item.ViewNum),
		})
	}
	response.Rows = rows
	response.Total = int(total)

	return response, nil
}

// UserGetHotArticle 获取热门文章
func (a *articleService) UserGetHotArticle(ctx context.Context) (*types.UserGetHotArticleResponse, error) {
	response := &types.UserGetHotArticleResponse{}
	articleIDZSet, err := a.redis.ZRevRangeWithScores(ctx, "article:view:ZSet", 0, 2).Result()
	if err != nil {
		a.logger.Error("failed to get article:view:ZSet", zap.Error(err))
		return nil, fmt.Errorf("failed to get article:view:ZSet, err: %w", err)
	}

	// 获取文章浏览量
	articleList := make([]types.GetHotArticleItem, 0)
	for _, z := range articleIDZSet {
		articleItem := types.GetHotArticleItem{}

		articleItem.ID = z.Member.(string)
		hashKey := "article:" + articleItem.ID + ":Hash"
		if exist := a.redis.Exists(ctx, hashKey); exist.Val() == 1 {
			fields := []string{"title", "viewNum"}
			result, err := a.redis.HMGet(ctx, hashKey, fields...).Result()
			if err != nil {
				a.logger.Error("failed to get hash data", zap.Error(err))
				return nil, fmt.Errorf("failed to get hash data, err: %w", err)
			}
			articleItem.Title = result[0].(string)
			if articleItem.ViewNum, err = strconv.Atoi(result[1].(string)); err != nil {
				a.logger.Error("parse int error", zap.Error(err))
				return nil, fmt.Errorf("parse int error, err: %w", err)
			}
		} else {
			// 查询MySQL
			id, err := strconv.ParseUint(articleItem.ID, 10, 64)
			if err != nil {
				a.logger.Error("parse uint64 error", zap.Error(err))
				return nil, fmt.Errorf("parse uint64 error, err: %w", err)
			}
			articleInfo, err := a.articleModel.GetArticleDetailByID(ctx, id)
			if err != nil {
				a.logger.Error("get article detail by id error", zap.Error(err))
				return nil, fmt.Errorf("get article detail by id error, err: %w", err)
			}

			// 设置缓存
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
			if err = a.redis.HMSet(ctx, "article:"+articleItem.ID+":Hash", mapData).Err(); err != nil {
				a.logger.Error("redis set article hash error", zap.Error(err))
				return nil, fmt.Errorf("redis set article hash error: %w", err)
			}

			articleItem.Title = articleInfo.Title
			articleItem.ViewNum = int(articleInfo.ViewNum)
		}
		articleList = append(articleList, articleItem)
	}
	response.Rows = articleList
	response.Total = len(articleList)

	return response, nil
}

// UserGetTimeline 获取文章归档
func (a *articleService) UserGetTimeline(ctx context.Context) (*types.GetTimelineResponse, error) {
	// ==============================================实现方式1: 循环读取redis的Hash结构==============================================
	//response := &types.GetTimelineResponse{}
	//articleIDZSet, err := a.redis.ZRevRangeWithScores(ctx, "article:time:ZSet", 0, -1).Result()
	//if err != nil {
	//	a.logger.Error("failed to get article:time:ZSet", zap.Error(err))
	//	return nil, fmt.Errorf("failed to get article:time:ZSet, err: %w", err)
	//}
	//groupedArticles := make(map[string][]types.GetTimelineListItem)
	//for _, z := range articleIDZSet {
	//	articleID := z.Member.(string)
	//	hashKey := "article:" + articleID + ":Hash"
	//	if exist := a.redis.Exists(ctx, hashKey); exist.Val() == 1 {
	//		result, err := a.redis.HMGet(ctx, hashKey, []string{"title", "createTime"}...).Result()
	//		if err != nil {
	//			a.logger.Error("failed to get hash data", zap.Error(err))
	//			return nil, fmt.Errorf("failed to get hash data, err: %w", err)
	//		}
	//		articleItem := types.GetTimelineListItem{
	//			ID:         articleID,
	//			Title:      result[0].(string),
	//			CreateTime: result[1].(string)[:16],
	//		}
	//		year := articleItem.CreateTime[:4]
	//		groupedArticles[year] = append(groupedArticles[year], articleItem)
	//	} else {
	//		// 查MySQL数据库
	//		id, err := strconv.ParseUint(articleID, 10, 64)
	//		if err != nil {
	//			a.logger.Error("parse uint64 error", zap.Error(err))
	//			return nil, err
	//		}
	//		articleInfo, err := a.articleModel.GetArticleDetailByID(ctx, id)
	//		if err != nil {
	//			a.logger.Error("get article detail by id error", zap.Error(err))
	//			return nil, err
	//		}
	//
	//		// 设置缓存
	//		mapData := map[string]interface{}{
	//			"id":         articleInfo.ID,
	//			"title":      articleInfo.Title,
	//			"describe":   articleInfo.Describe,
	//			"content":    articleInfo.Content,
	//			"viewNum":    articleInfo.ViewNum,
	//			"createTime": articleInfo.CreateTime.Format(constants.TimeLayoutToSecond),
	//			"updateTime": articleInfo.UpdateTime.Format(constants.TimeLayoutToSecond),
	//			"tagID":      articleInfo.TagID,
	//			"tagName":    articleInfo.TagName,
	//		}
	//		if err = a.redis.HMSet(ctx, "article:"+articleID+":Hash", mapData).Err(); err != nil {
	//			a.logger.Error("redis set article hash error", zap.Error(err))
	//			return nil, fmt.Errorf("redis set article hash error: %w", err)
	//		}
	//
	//		articleItem := types.GetTimelineListItem{
	//			ID:         articleID,
	//			Title:      articleInfo.Title,
	//			CreateTime: articleInfo.CreateTime.Format(constants.TimeLayoutToMinute),
	//		}
	//		groupedArticles[articleItem.CreateTime[:4]] = append(groupedArticles[articleItem.CreateTime[:4]], articleItem)
	//	}
	//}
	//var rows []types.GetTimelineRowsItem
	//years := make([]string, 0, len(groupedArticles))
	//for year := range groupedArticles {
	//	years = append(years, year)
	//}
	//sort.Slice(years, func(i, j int) bool {
	//	return years[i] > years[j]
	//})
	//for _, year := range years {
	//	rows = append(rows, types.GetTimelineRowsItem{
	//		Time: year,
	//		List: groupedArticles[year],
	//	})
	//}
	//response.Rows = rows
	//response.Total = len(articleIDZSet)

	// ==============================================实现方式2：使用Pipeline==============================================
	// 1. 从Redis获取有序集合
	articleIDs, err := a.redis.ZRevRange(ctx, "article:time:ZSet", 0, -1).Result()
	if err != nil {
		a.logger.Error("failed to get article:time:ZSet", zap.Error(err))
		return nil, fmt.Errorf("failed to get article:time:ZSet: %w", err)
	}

	// 2. 批量处理文章数据
	groupedArticles := make(map[string][]types.GetTimelineListItem)
	missingIDs := make([]string, 0)
	cachedArticles := make(map[string]types.GetTimelineListItem)

	// 批量检查缓存存在性
	hashKeys := make([]string, len(articleIDs))
	for i, id := range articleIDs {
		hashKeys[i] = "article:" + id + ":Hash"
	}
	exists, _ := a.redis.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		for _, key := range hashKeys {
			pipe.Exists(ctx, key)
		}
		return nil
	})

	// 批量获取缓存存在的数据
	pipeline := a.redis.Pipeline()
	for i, e := range exists {
		if e.(*redis.IntCmd).Val() == 1 {
			pipeline.HMGet(ctx, hashKeys[i], "title", "createTime")
		} else {
			missingIDs = append(missingIDs, articleIDs[i])
		}
	}
	cachedResults, _ := pipeline.Exec(ctx)

	// 3. 处理缓存数据
	resultIndex := 0
	for i, id := range articleIDs {
		if exists[i].(*redis.IntCmd).Val() == 1 {
			res := cachedResults[resultIndex].(*redis.SliceCmd).Val()
			resultIndex++

			if res[0] != nil && res[1] != nil {
				item := types.GetTimelineListItem{
					ID:         id,
					Title:      res[0].(string),
					CreateTime: res[1].(string)[:16],
				}
				cachedArticles[id] = item
			}
		}
	}

	// 4. 批量处理缺失数据
	if len(missingIDs) > 0 {
		ids := make([]uint64, len(missingIDs))
		for i, idStr := range missingIDs {
			id, _ := strconv.ParseUint(idStr, 10, 64)
			ids[i] = id
		}

		articles, err := a.articleModel.GetArticleListByIDList(ctx, ids)
		if err != nil {
			a.logger.Error("get articles by ids error", zap.Error(err))
			return nil, err
		}

		// 批量设置缓存
		pipe := a.redis.Pipeline()
		for _, article := range articles {
			idStr := strconv.FormatUint(article.ID, 10)
			item := types.GetTimelineListItem{
				ID:         idStr,
				Title:      article.Title,
				CreateTime: article.CreateTime.Format(constants.TimeLayoutToMinute),
			}
			cachedArticles[idStr] = item

			mapData := map[string]interface{}{
				"title":      article.Title,
				"createTime": article.CreateTime.Format(constants.TimeLayoutToSecond),
			}
			pipe.HMSet(ctx, "article:"+idStr+":Hash", mapData)
		}
		_, err = pipe.Exec(ctx)
		if err != nil {
			a.logger.Error("failed to exec pipeline", zap.Error(err))
			return nil, fmt.Errorf("failed to exec pipeline: %w", err)
		}
	}

	// 5. 按年份分组
	for _, id := range articleIDs {
		if item, ok := cachedArticles[id]; ok {
			year := item.CreateTime[:4]
			groupedArticles[year] = append(groupedArticles[year], item)
		}
	}

	// 6. 构建响应
	response := &types.GetTimelineResponse{Total: len(articleIDs)}
	years := make([]string, 0, len(groupedArticles))
	for year := range groupedArticles {
		years = append(years, year)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(years)))

	for _, year := range years {
		response.Rows = append(response.Rows, types.GetTimelineRowsItem{
			Time: year,
			List: groupedArticles[year],
		})
	}

	return response, nil
}

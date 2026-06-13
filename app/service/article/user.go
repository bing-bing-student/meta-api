package article

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"meta-api/common/cachekey"
	"meta-api/common/constants"
	"meta-api/common/idutil"
	"meta-api/common/types"
)

// UserGetArticleList 获取文章列表
func (a *articleService) UserGetArticleList(ctx context.Context,
	request *types.UserGetArticleListRequest) (*types.UserGetArticleListResponse, error) {

	start := (request.Page - 1) * request.PageSize
	stop := start + request.PageSize - 1

	// 获取文章 ID 有序集合
	articleIDZSet, err := a.redis.ZRevRangeWithScores(ctx, cachekey.ArticleTimeZSet().String(), int64(start), int64(stop)).Result()
	if err != nil {
		a.logger.Error("failed to get article:time:ZSet", zap.Error(err))
		return nil, err
	}

	articleList := make([]types.UserGetArticleItem, 0)
	for _, z := range articleIDZSet {
		articleItem := types.UserGetArticleItem{}

		articleItem.ID = z.Member.(string)
		hashKey := cachekey.ArticleHash(articleItem.ID).String()
		if exist := a.redis.Exists(ctx, hashKey); exist.Val() == 1 {
			// 获取缓存数据
			fields := []string{"title", "tagName", "describe", "createTime", "updateTime", "viewNum"}
			result, err := a.redis.HMGet(ctx, hashKey, fields...).Result()
			if err != nil {
				a.logger.Error("get article info HMGet error", zap.Error(err))
				return nil, fmt.Errorf("get article info HMGet error, err: %w", err)
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
			id, err := idutil.ParseID("articleID", articleItem.ID)
			if err != nil {
				a.logger.Error("invalid article id", zap.Error(err))
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
			if err = a.redis.HMSet(ctx, cachekey.ArticleHash(articleItem.ID).String(), mapData).Err(); err != nil {
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
	response := &types.UserGetArticleListResponse{}
	response.Rows = articleList
	response.Total = int(a.redis.ZCard(ctx, cachekey.ArticleTimeZSet().String()).Val())
	return response, nil
}

// UserGetArticleDetail 获取文章详情
func (a *articleService) UserGetArticleDetail(ctx context.Context,
	request *types.UserGetArticleDetailRequest) (*types.UserGetArticleDetailResponse, error) {

	response := &types.UserGetArticleDetailResponse{}
	hashKey := cachekey.ArticleHash(request.ID).String()
	if exist := a.redis.Exists(ctx, hashKey); exist.Val() == 1 {
		// 缓存查询
		fields := []string{"title", "tagName", "content", "createTime", "updateTime"}
		result, err := a.redis.HMGet(ctx, hashKey, fields...).Result()
		if err != nil {
			a.logger.Error("get article info HMGet error", zap.Error(err))
			return nil, err
		}
		response.ID = request.ID
		response.Title = result[0].(string)
		response.TagName = result[1].(string)
		response.Content = result[2].(string)
		response.CreateTime = result[3].(string)[:10]
		response.UpdateTime = result[4].(string)[:10]
	} else {
		// 查询 MySQL
		id, err := idutil.ParseID("articleID", request.ID)
		if err != nil {
			a.logger.Error("invalid article id", zap.Error(err))
			return nil, err
		}
		articleInfo, err := a.articleModel.GetArticleDetailByID(ctx, id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, fmt.Errorf("record not found: %w", err)
			}
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
		if err = a.redis.HMSet(ctx, cachekey.ArticleHash(request.ID).String(), mapData).Err(); err != nil {
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

	return response, nil
}

// UserSearchArticle 搜索文章
//
// 注意：MySQL 中的 article.view_num 由 cron 周期性回写，会落后于 Redis 中的真实浏览量
// （热路径只 +1 到 article:view:ZSet 与 article:{id}:Hash）。直接返回 MySQL 的 view_num
// 会与文章详情页 / 热门文章列表的浏览量不一致。
//
// 这里在 MySQL 检索结果之上，用 article:view:ZSet 中的 score 校正每条结果的 view_num，
// ZSet 不存在该 member 时（极少见，比如缓存预热未覆盖到）才回退到 MySQL 的值兜底。
func (a *articleService) UserSearchArticle(ctx context.Context,
	request *types.UserSearchArticleRequest) (*types.UserSearchArticleResponse, error) {

	limit := request.PageSize
	offset := (request.Page - 1) * request.PageSize
	word := strings.TrimSpace(request.Word)
	articleList, total, err := a.articleModel.SearchArticle(ctx, word, limit, offset)
	if err != nil {
		a.logger.Error("failed to search article", zap.Error(err))
		return nil, fmt.Errorf("failed to search article, err: %w", err)
	}

	// 用 Redis ZSet 中的 score 校正浏览量，pipeline 一次拿到所有结果
	// 用 ZScore 而不是 ZMScore，方便通过 redis.Nil 区分「不存在」与「分数恰好为 0」
	viewZSetKey := cachekey.ArticleViewZSet().String()
	scoreCmds := make([]*redis.FloatCmd, len(articleList))
	if len(articleList) > 0 {
		_, pipeErr := a.redis.Pipelined(ctx, func(pipe redis.Pipeliner) error {
			for i, item := range articleList {
				scoreCmds[i] = pipe.ZScore(ctx, viewZSetKey, strconv.FormatUint(item.ID, 10))
			}
			return nil
		})
		// pipeline 整体失败仅记录日志、降级使用 MySQL 的 view_num，不阻塞搜索
		if pipeErr != nil && !errors.Is(pipeErr, redis.Nil) {
			a.logger.Warn("failed to pipeline ZScore for view num correction",
				zap.Error(pipeErr))
		}
	}

	rows := make([]types.UserGetArticleItem, 0, len(articleList))
	for i, item := range articleList {
		viewNum := int(item.ViewNum)
		if scoreCmds[i] != nil {
			if score, scoreErr := scoreCmds[i].Result(); scoreErr == nil {
				viewNum = int(score)
			} else if !errors.Is(scoreErr, redis.Nil) {
				// 单条失败（非 not-found）仅打日志，不影响该条返回
				a.logger.Warn("zscore failed for article",
					zap.Uint64("articleID", item.ID), zap.Error(scoreErr))
			}
		}

		rows = append(rows, types.UserGetArticleItem{
			ID:         strconv.Itoa(int(item.ID)),
			Title:      item.Title,
			Describe:   item.Describe,
			ViewNum:    viewNum,
			CreateTime: item.CreateTime.Format(constants.TimeLayoutToDay),
		})
	}
	response := &types.UserSearchArticleResponse{}
	response.Rows = rows
	response.Total = int(total)

	return response, nil
}

// UserGetHotArticle 获取热门文章
func (a *articleService) UserGetHotArticle(ctx context.Context) (*types.UserGetHotArticleResponse, error) {
	articleIDZSet, err := a.redis.ZRevRangeWithScores(ctx, cachekey.ArticleViewZSet().String(), 0, 2).Result()
	if err != nil {
		a.logger.Error("failed to get article:view:ZSet", zap.Error(err))
		return nil, fmt.Errorf("failed to get article:view:ZSet, err: %w", err)
	}

	// 获取文章浏览量
	articleList := make([]types.GetHotArticleItem, 0)
	for _, z := range articleIDZSet {
		articleItem := types.GetHotArticleItem{}

		articleItem.ID = z.Member.(string)
		hashKey := cachekey.ArticleHash(articleItem.ID).String()
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
			// 查询 MySQL
			id, err := idutil.ParseID("articleID", articleItem.ID)
			if err != nil {
				a.logger.Error("invalid article id", zap.Error(err))
				return nil, fmt.Errorf("invalid article id, err: %w", err)
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
			if err = a.redis.HMSet(ctx, cachekey.ArticleHash(articleItem.ID).String(), mapData).Err(); err != nil {
				a.logger.Error("redis set article hash error", zap.Error(err))
				return nil, fmt.Errorf("redis set article hash error: %w", err)
			}

			articleItem.Title = articleInfo.Title
			articleItem.ViewNum = int(articleInfo.ViewNum)
		}
		articleList = append(articleList, articleItem)
	}
	response := &types.UserGetHotArticleResponse{}
	response.Rows = articleList
	response.Total = len(articleList)

	return response, nil
}

// UserGetTimeline 获取文章归档
func (a *articleService) UserGetTimeline(ctx context.Context) (*types.GetTimelineResponse, error) {
	// ==============================================实现方式1: 循环读取redis的Hash结构==============================================
	response := &types.GetTimelineResponse{}
	articleIDZSet, err := a.redis.ZRevRangeWithScores(ctx, cachekey.ArticleTimeZSet().String(), 0, -1).Result()
	if err != nil {
		a.logger.Error("failed to get article:time:ZSet", zap.Error(err))
		return nil, fmt.Errorf("failed to get article:time:ZSet, err: %w", err)
	}
	groupedArticles := make(map[string][]types.GetTimelineListItem)
	for _, z := range articleIDZSet {
		articleID := z.Member.(string)
		hashKey := cachekey.ArticleHash(articleID).String()
		if exist := a.redis.Exists(ctx, hashKey); exist.Val() == 1 {
			result, err := a.redis.HMGet(ctx, hashKey, []string{"title", "createTime"}...).Result()
			if err != nil {
				a.logger.Error("failed to get hash data", zap.Error(err))
				return nil, fmt.Errorf("failed to get hash data, err: %w", err)
			}
			articleItem := types.GetTimelineListItem{
				ID:         articleID,
				Title:      result[0].(string),
				CreateTime: result[1].(string)[:16],
			}
			year := articleItem.CreateTime[:4]
			groupedArticles[year] = append(groupedArticles[year], articleItem)
		} else {
			// 查 MySQL 数据库
			id, err := idutil.ParseID("articleID", articleID)
			if err != nil {
				a.logger.Error("invalid article id", zap.Error(err))
				return nil, err
			}
			articleInfo, err := a.articleModel.GetArticleDetailByID(ctx, id)
			if err != nil {
				a.logger.Error("get article detail by id error", zap.Error(err))
				return nil, err
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
			if err = a.redis.HMSet(ctx, cachekey.ArticleHash(articleID).String(), mapData).Err(); err != nil {
				a.logger.Error("redis set article hash error", zap.Error(err))
				return nil, fmt.Errorf("redis set article hash error: %w", err)
			}

			articleItem := types.GetTimelineListItem{
				ID:         articleID,
				Title:      articleInfo.Title,
				CreateTime: articleInfo.CreateTime.Format(constants.TimeLayoutToMinute),
			}
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
	response.Rows = rows
	response.Total = len(articleIDZSet)

	return response, nil
}

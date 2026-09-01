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

	articleIDZSet, total, err := a.readArticlePage(ctx, cachekey.ArticleTimeZSet().String(), start, stop)
	if err != nil {
		a.logger.Error("failed to get article:time:ZSet", zap.Error(err))
		return nil, err
	}
	articleIDs := make([]string, 0, len(articleIDZSet))
	for _, z := range articleIDZSet {
		articleID, ok := z.Member.(string)
		if !ok {
			return nil, fmt.Errorf("invalid article cache member type %T", z.Member)
		}
		articleIDs = append(articleIDs, articleID)
	}
	entries, misses, err := a.readArticleListCache(ctx, articleIDs)
	if err != nil {
		return nil, err
	}
	loaded, err := a.loadArticleListMisses(ctx, misses)
	if err != nil {
		return nil, err
	}
	for id, entry := range loaded {
		entries[id] = entry
	}

	rows := make([]types.UserGetArticleItem, 0, len(articleIDs))
	for _, articleID := range articleIDs {
		entry, exists := entries[articleID]
		if !exists {
			return nil, fmt.Errorf("article %s not found", articleID)
		}
		rows = append(rows, types.UserGetArticleItem{
			ID:         articleID,
			Title:      entry.Title,
			TagName:    entry.TagName,
			Describe:   entry.Describe,
			CreateTime: formatCachedArticleTime(entry.CreateTime, constants.TimeLayoutToDay),
			UpdateTime: formatCachedArticleTime(entry.UpdateTime, constants.TimeLayoutToDay),
			ViewNum:    entry.ViewNum,
		})
	}
	return &types.UserGetArticleListResponse{Rows: rows, Total: int(total)}, nil
}

// UserGetArticleDetail 获取文章详情
func (a *articleService) UserGetArticleDetail(ctx context.Context,
	request *types.UserGetArticleDetailRequest) (*types.UserGetArticleDetailResponse, error) {

	response := &types.UserGetArticleDetailResponse{}
	hashKey := cachekey.ArticleHash(request.ID).String()
	fields := []string{"title", "tagName", "content", "createTime", "updateTime"}
	result, err := a.redis.HMGet(ctx, hashKey, fields...).Result()
	if err != nil {
		a.logger.Error("get article info HMGet error", zap.Error(err))
		return nil, err
	}
	cached, cacheHit := redisStringFields(result, len(fields))
	if cacheHit {
		response.ID = request.ID
		response.Title = cached[0]
		response.TagName = cached[1]
		response.Content = cached[2]
		response.CreateTime = formatCachedArticleTime(cached[3], constants.TimeLayoutToDay)
		response.UpdateTime = formatCachedArticleTime(cached[4], constants.TimeLayoutToDay)
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
		mapData := map[string]any{
			"id":         articleInfo.ID,
			"title":      articleInfo.Title,
			"describe":   articleInfo.Describe,
			"content":    articleInfo.Content,
			"createTime": articleInfo.CreateTime.Format(constants.TimeLayoutToSecond),
			"updateTime": articleInfo.UpdateTime.Format(constants.TimeLayoutToSecond),
			"tagID":      articleTagIDValue(articleInfo.TagID),
			"tagName":    articleInfo.TagName,
		}
		if err = a.redis.HSet(ctx, hashKey, mapData).Err(); err != nil {
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
// （热路径只 +1 到 article:view:ZSet）。直接返回 MySQL 的 view_num
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
	articleIDs := make([]string, 0, len(articleIDZSet))
	for _, z := range articleIDZSet {
		articleID, ok := z.Member.(string)
		if !ok {
			return nil, fmt.Errorf("invalid article cache member type %T", z.Member)
		}
		articleIDs = append(articleIDs, articleID)
	}
	entries, misses, err := a.readArticleListCache(ctx, articleIDs)
	if err != nil {
		return nil, err
	}
	loaded, err := a.loadArticleListMisses(ctx, misses)
	if err != nil {
		return nil, err
	}
	for id, entry := range loaded {
		entries[id] = entry
	}
	rows := make([]types.GetHotArticleItem, 0, len(articleIDs))
	for _, articleID := range articleIDs {
		entry, exists := entries[articleID]
		if !exists {
			return nil, fmt.Errorf("article %s not found", articleID)
		}
		rows = append(rows, types.GetHotArticleItem{
			ID: articleID, Title: entry.Title, ViewNum: entry.ViewNum,
		})
	}
	return &types.UserGetHotArticleResponse{Rows: rows, Total: len(rows)}, nil
}

// UserGetTimeline 获取文章归档
func (a *articleService) UserGetTimeline(ctx context.Context) (*types.GetTimelineResponse, error) {
	response := &types.GetTimelineResponse{}
	articleIDZSet, err := a.redis.ZRevRangeWithScores(ctx, cachekey.ArticleTimeZSet().String(), 0, -1).Result()
	if err != nil {
		a.logger.Error("failed to get article:time:ZSet", zap.Error(err))
		return nil, fmt.Errorf("failed to get article:time:ZSet, err: %w", err)
	}
	articleIDs := make([]string, 0, len(articleIDZSet))
	for _, z := range articleIDZSet {
		articleID, ok := z.Member.(string)
		if !ok {
			return nil, fmt.Errorf("invalid article cache member type %T", z.Member)
		}
		articleIDs = append(articleIDs, articleID)
	}
	entries, misses, err := a.readArticleListCache(ctx, articleIDs)
	if err != nil {
		return nil, err
	}
	loaded, err := a.loadArticleListMisses(ctx, misses)
	if err != nil {
		return nil, err
	}
	for id, entry := range loaded {
		entries[id] = entry
	}

	groupedArticles := make(map[string][]types.GetTimelineListItem)
	for _, articleID := range articleIDs {
		entry, exists := entries[articleID]
		if !exists {
			return nil, fmt.Errorf("article %s not found", articleID)
		}
		createTime := formatCachedArticleTime(entry.CreateTime, constants.TimeLayoutToMinute)
		if len(createTime) < 4 {
			return nil, fmt.Errorf("invalid cached create time for article %s", articleID)
		}
		item := types.GetTimelineListItem{ID: articleID, Title: entry.Title, CreateTime: createTime}
		groupedArticles[createTime[:4]] = append(groupedArticles[createTime[:4]], item)
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

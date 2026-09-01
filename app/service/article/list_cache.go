package article

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/redis/go-redis/v9"

	articleModel "meta-api/app/model/article"
	"meta-api/common/cachekey"
	"meta-api/common/constants"
)

var articleListHashFields = []string{"title", "tagName", "describe", "createTime", "updateTime"}

type articleListCacheEntry struct {
	ID         string
	Title      string
	TagName    string
	Describe   string
	CreateTime string
	UpdateTime string
	ViewNum    int
}

// readArticlePage 把分页 ID 与总数放进同一 Pipeline，避免两个独立 RTT。
func (a *articleService) readArticlePage(ctx context.Context, key string, start, stop int) ([]redis.Z, int64, error) {
	pipe := a.redis.Pipeline()
	rowsCmd := pipe.ZRevRangeWithScores(ctx, key, int64(start), int64(stop))
	totalCmd := pipe.ZCard(ctx, key)
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, 0, err
	}
	rows, err := rowsCmd.Result()
	if err != nil {
		return nil, 0, err
	}
	total, err := totalCmd.Result()
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// readArticleListCache 批量读取文章列表需要的 Hash 字段及实时浏览量。
// HMGET 对不存在的 Key 返回 nil 字段，因此不再额外执行 EXISTS。
func (a *articleService) readArticleListCache(ctx context.Context,
	articleIDs []string,
) (map[string]articleListCacheEntry, []string, error) {
	entries := make(map[string]articleListCacheEntry, len(articleIDs))
	if len(articleIDs) == 0 {
		return entries, nil, nil
	}

	pipe := a.redis.Pipeline()
	hashCmds := make([]*redis.SliceCmd, len(articleIDs))
	viewCmds := make([]*redis.FloatCmd, len(articleIDs))
	viewKey := cachekey.ArticleViewZSet().String()
	for i, articleID := range articleIDs {
		hashCmds[i] = pipe.HMGet(ctx, cachekey.ArticleHash(articleID).String(), articleListHashFields...)
		viewCmds[i] = pipe.ZScore(ctx, viewKey, articleID)
	}
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return nil, nil, err
	}

	misses := make([]string, 0)
	for i, articleID := range articleIDs {
		values, hashErr := hashCmds[i].Result()
		view, viewErr := viewCmds[i].Result()
		entry, ok := decodeArticleListCacheEntry(articleID, values, view)
		if hashErr != nil || (viewErr != nil && !errors.Is(viewErr, redis.Nil)) {
			return nil, nil, errors.Join(hashErr, viewErr)
		}
		if !ok || errors.Is(viewErr, redis.Nil) {
			misses = append(misses, articleID)
			continue
		}
		entries[articleID] = entry
	}
	return entries, misses, nil
}

func decodeArticleListCacheEntry(articleID string, values []any, view float64) (articleListCacheEntry, bool) {
	fields, ok := redisStringFields(values, len(articleListHashFields))
	if !ok {
		return articleListCacheEntry{}, false
	}
	return articleListCacheEntry{
		ID: articleID, Title: fields[0], TagName: fields[1], Describe: fields[2],
		CreateTime: fields[3], UpdateTime: fields[4], ViewNum: int(view),
	}, true
}

func redisStringFields(values []any, expected int) ([]string, bool) {
	if len(values) != expected {
		return nil, false
	}
	fields := make([]string, len(values))
	for i, value := range values {
		text, ok := value.(string)
		if !ok {
			return nil, false
		}
		fields[i] = text
	}
	return fields, true
}

// loadArticleListMisses 一次查询补齐所有缓存缺失项，并分批回填 Hash。
func (a *articleService) loadArticleListMisses(ctx context.Context, ids []string) (map[string]articleListCacheEntry, error) {
	entries := make(map[string]articleListCacheEntry, len(ids))
	if len(ids) == 0 {
		return entries, nil
	}
	numericIDs := make([]uint64, 0, len(ids))
	for _, rawID := range ids {
		id, err := strconv.ParseUint(rawID, 10, 64)
		if err != nil || id == 0 {
			return nil, fmt.Errorf("invalid article id %q", rawID)
		}
		numericIDs = append(numericIDs, id)
	}
	articles, err := a.articleModel.GetArticleListByIDList(ctx, numericIDs)
	if err != nil {
		return nil, err
	}

	pipe := a.redis.Pipeline()
	viewKey := cachekey.ArticleViewZSet().String()
	viewCmds := make(map[string]*redis.FloatCmd, len(articles))
	for _, item := range articles {
		if item == nil {
			continue
		}
		articleID := strconv.FormatUint(item.ID, 10)
		entry := articleListCacheEntry{
			ID: articleID, Title: item.Title, TagName: item.Tag.Name, Describe: item.Describe,
			CreateTime: item.CreateTime.Format(constants.TimeLayoutToSecond),
			UpdateTime: item.UpdateTime.Format(constants.TimeLayoutToSecond),
			ViewNum:    int(item.ViewNum),
		}
		entries[articleID] = entry
		pipe.HSet(ctx, cachekey.ArticleHash(articleID).String(), articleCacheMap(item))
		// 页面 ID 来自另一个排序集合，但浏览量成员可能因缓存不完整而缺失；
		// NX 只修复缺失项，不覆盖已经由浏览打点累计出的实时值。
		pipe.ZAddNX(ctx, viewKey, redis.Z{Score: float64(item.ViewNum), Member: articleID})
		viewCmds[articleID] = pipe.ZScore(ctx, viewKey, articleID)
	}
	if _, err = pipe.Exec(ctx); err != nil {
		return nil, err
	}
	for articleID, cmd := range viewCmds {
		view, viewErr := cmd.Result()
		if viewErr != nil {
			return nil, viewErr
		}
		entry := entries[articleID]
		entry.ViewNum = int(view)
		entries[articleID] = entry
	}
	return entries, nil
}

func articleCacheMap(item *articleModel.Article) map[string]any {
	tagID := uint64(0)
	if item.TagID != nil {
		tagID = *item.TagID
	}
	return map[string]any{
		"id":         item.ID,
		"title":      item.Title,
		"describe":   item.Describe,
		"content":    item.Content,
		"createTime": item.CreateTime.Format(constants.TimeLayoutToSecond),
		"updateTime": item.UpdateTime.Format(constants.TimeLayoutToSecond),
		"tagID":      tagID,
		"tagName":    item.Tag.Name,
	}
}

func formatCachedArticleTime(value, layout string) string {
	if len(value) <= len(layout) {
		return value
	}
	return value[:len(layout)]
}

package article

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

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

// readArticlePage 读取指定排序维度的文章分页。
// order 只接受 time 或 view，start/stop 使用 Redis ZREVRANGE 的闭区间语义。
// 返回值依次是当前页成员、文章总数和错误。当任一文章排序 ZSet 缺失或
// Redis 不可用时，会从 MySQL 读取完整快照；Redis 可用时只重建缺失索引，
// 当前请求即使缓存回填失败也仍返回 MySQL 结果。
func (a *articleService) readArticlePage(ctx context.Context, order string, start, stop int) ([]redis.Z, int64, error) {
	key, ok := cachekey.ArticleOrderZSet(order)
	if !ok {
		return nil, 0, fmt.Errorf("invalid article order: %s", order)
	}

	rows, total, timeExists, viewExists, cacheErr := a.readArticlePageCache(ctx, key.String(), start, stop)
	if cacheErr == nil && timeExists && viewExists {
		return rows, total, nil
	}

	// 缓存缺失时串行回源，避免同一进程内的并发请求同时查询 MySQL 和重建缓存。
	a.articleCacheMu.Lock()
	defer a.articleCacheMu.Unlock()

	// 只有缓存读取成功但 Key 缺失时才重新检查；等待锁期间可能已有请求完成重建。
	if cacheErr == nil {
		rows, total, timeExists, viewExists, cacheErr = a.readArticlePageCache(ctx, key.String(), start, stop)
		if cacheErr == nil && timeExists && viewExists {
			return rows, total, nil
		}
	}

	list, dbErr := a.articleModel.ListTimeAndView(ctx)
	requestedExists := articleOrderCacheExists(order, timeExists, viewExists)
	if dbErr != nil {
		// 请求所需的 ZSet 仍完整时，不因另一个排序索引修复失败而阻塞本次读取。
		if cacheErr == nil && requestedExists {
			a.logger.Warn("failed to repair companion article order cache", zap.Error(dbErr))
			return rows, total, nil
		}
		if cacheErr != nil {
			return nil, 0, errors.Join(cacheErr, dbErr)
		}
		return nil, 0, dbErr
	}

	replaceTime := cacheErr != nil || !timeExists
	replaceView := cacheErr != nil || !viewExists
	if rebuildErr := a.replaceArticleOrderCaches(ctx, list, replaceTime, replaceView); rebuildErr != nil {
		a.logger.Warn("failed to rebuild article order cache; serving MySQL fallback", zap.String("order", order), zap.Error(rebuildErr))
	} else {
		a.logger.Info("article order cache rebuilt from MySQL", zap.Bool("time", replaceTime), zap.Bool("view", replaceView), zap.Int("total", len(list)))
	}

	// 所需索引原本存在时保留其中尚未持久化的实时浏览量排序结果；这里只修复伴随索引。
	if cacheErr == nil && requestedExists {
		return rows, total, nil
	}
	return articleOrderPageFromSnapshot(list, order, start, stop)
}

// readArticlePageCache 在一次 Pipeline 中读取分页、总数和两个排序索引的存在状态。
// 返回的 timeExists/viewExists 用于区分“空结果”和“缓存 Key 已丢失”。
func (a *articleService) readArticlePageCache(ctx context.Context, key string, start, stop int) ([]redis.Z, int64, bool, bool, error) {
	pipe := a.redis.Pipeline()
	rowsCmd := pipe.ZRevRangeWithScores(ctx, key, int64(start), int64(stop))
	totalCmd := pipe.ZCard(ctx, key)
	timeExistsCmd := pipe.Exists(ctx, cachekey.ArticleTimeZSet().String())
	viewExistsCmd := pipe.Exists(ctx, cachekey.ArticleViewZSet().String())
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, 0, false, false, err
	}
	rows, err := rowsCmd.Result()
	if err != nil {
		return nil, 0, false, false, err
	}
	total, err := totalCmd.Result()
	if err != nil {
		return nil, 0, false, false, err
	}
	timeExists, err := timeExistsCmd.Result()
	if err != nil {
		return nil, 0, false, false, err
	}
	viewExists, err := viewExistsCmd.Result()
	if err != nil {
		return nil, 0, false, false, err
	}
	return rows, total, timeExists > 0, viewExists > 0, nil
}

func articleOrderCacheExists(order string, timeExists, viewExists bool) bool {
	if order == cachekey.OrderTime {
		return timeExists
	}
	return viewExists
}

// articleOrderPageFromSnapshot 把 MySQL 快照转换为与 ZREVRANGE 一致的倒序分页结果。
// score 相同时按 member 倒序排列，保证回源结果和 Redis 的顺序保持一致。
func articleOrderPageFromSnapshot(list []articleModel.TimeAndViewZSet, order string, start, stop int) ([]redis.Z, int64, error) {
	members := make([]redis.Z, 0, len(list))
	for _, item := range list {
		score := cachekey.ArticleTimeScore(item.CreateTime)
		if order == cachekey.OrderView {
			score = cachekey.ArticleViewScore(item.ViewNum)
		}
		members = append(members, redis.Z{
			Score:  score,
			Member: strconv.FormatUint(item.ID, 10),
		})
	}
	sort.Slice(members, func(i, j int) bool {
		if members[i].Score != members[j].Score {
			return members[i].Score > members[j].Score
		}
		return members[i].Member.(string) > members[j].Member.(string)
	})

	total := int64(len(members))
	if start < 0 {
		start = 0
	}
	if start >= len(members) {
		return []redis.Z{}, total, nil
	}
	if stop < 0 || stop >= len(members) {
		stop = len(members) - 1
	}
	if stop < start {
		return []redis.Z{}, total, nil
	}
	return members[start : stop+1], total, nil
}

// readArticleListCache 批量读取文章列表需要的 Hash 字段及实时浏览量。
// HMGET 对不存在的 Key 返回 nil 字段，因此不再额外执行 EXISTS。
func (a *articleService) readArticleListCache(ctx context.Context, articleIDs []string) (map[string]articleListCacheEntry, []string, error) {
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
		a.logger.Warn("failed to read article list cache; loading rows from MySQL", zap.Error(err))
		return entries, append([]string(nil), articleIDs...), nil
	}

	misses := make([]string, 0)
	for i, articleID := range articleIDs {
		values, hashErr := hashCmds[i].Result()
		view, viewErr := viewCmds[i].Result()
		entry, ok := decodeArticleListCacheEntry(articleID, values, view)
		if (hashErr != nil && !errors.Is(hashErr, redis.Nil)) ||
			(viewErr != nil && !errors.Is(viewErr, redis.Nil)) {
			a.logger.Warn("failed to read cached article row; loading it from MySQL",
				zap.String("article_id", articleID), zap.Error(errors.Join(hashErr, viewErr)))
			misses = append(misses, articleID)
			continue
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
		// 只读取实时浏览量，不在这里补写 ZSet。排序索引必须由完整快照重建，
		// 否则缓存整体丢失时会被当前一页数据误建成“看似存在”的残缺集合。
		viewCmds[articleID] = pipe.ZScore(ctx, cachekey.ArticleViewZSet().String(), articleID)
	}
	if _, cacheErr := pipe.Exec(ctx); cacheErr != nil && !errors.Is(cacheErr, redis.Nil) {
		a.logger.Warn("failed to refill article hashes; serving MySQL fallback", zap.Error(cacheErr))
		return entries, nil
	}
	for articleID, cmd := range viewCmds {
		view, viewErr := cmd.Result()
		if errors.Is(viewErr, redis.Nil) {
			continue
		}
		if viewErr != nil {
			a.logger.Warn("failed to read live article view count; using MySQL value",
				zap.String("article_id", articleID), zap.Error(viewErr))
			continue
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

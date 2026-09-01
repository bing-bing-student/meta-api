package tag

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

var tagArticleHashFields = []string{"title", "describe", "createTime", "updateTime"}

type tagArticleCacheEntry struct {
	ID         string
	Title      string
	Describe   string
	CreateTime string
	UpdateTime string
	ViewNum    int
}

func articleTagIDValue(tagID *uint64) uint64 {
	if tagID == nil {
		return 0
	}
	return *tagID
}

func (t *tagService) readTagArticlePage(ctx context.Context, key string,
	start, stop int,
) ([]string, int64, error) {
	pipe := t.redis.Pipeline()
	rowsCmd := pipe.ZRevRange(ctx, key, int64(start), int64(stop))
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

func (t *tagService) loadTagArticlePage(ctx context.Context, key, tagName string,
	start, stop int,
) ([]string, int64, error) {
	rows, total, err := t.readTagArticlePage(ctx, key, start, stop)
	if err != nil || total > 0 {
		return rows, total, err
	}
	articles, err := t.tagModel.GetArticleListByTagName(ctx, tagName)
	if err != nil {
		return nil, 0, err
	}
	if len(articles) == 0 {
		return nil, 0, fmt.Errorf("not found tagName")
	}
	members := make([]redis.Z, 0, len(articles))
	for _, item := range articles {
		members = append(members, redis.Z{
			Score: cachekey.ArticleTimeScore(item.CreateTime), Member: item.ID,
		})
	}
	if err = t.redis.ZAdd(ctx, key, members...).Err(); err != nil {
		return nil, 0, err
	}
	return t.readTagArticlePage(ctx, key, start, stop)
}

func (t *tagService) readTagArticleCache(ctx context.Context,
	articleIDs []string,
) (map[string]tagArticleCacheEntry, []string, error) {
	entries := make(map[string]tagArticleCacheEntry, len(articleIDs))
	if len(articleIDs) == 0 {
		return entries, nil, nil
	}
	pipe := t.redis.Pipeline()
	hashCmds := make([]*redis.SliceCmd, len(articleIDs))
	viewCmds := make([]*redis.FloatCmd, len(articleIDs))
	viewKey := cachekey.ArticleViewZSet().String()
	for i, articleID := range articleIDs {
		hashCmds[i] = pipe.HMGet(ctx, cachekey.ArticleHash(articleID).String(), tagArticleHashFields...)
		viewCmds[i] = pipe.ZScore(ctx, viewKey, articleID)
	}
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return nil, nil, err
	}

	misses := make([]string, 0)
	for i, articleID := range articleIDs {
		values, hashErr := hashCmds[i].Result()
		view, viewErr := viewCmds[i].Result()
		if hashErr != nil || (viewErr != nil && !errors.Is(viewErr, redis.Nil)) {
			return nil, nil, errors.Join(hashErr, viewErr)
		}
		fields, ok := tagArticleStringFields(values)
		if !ok || errors.Is(viewErr, redis.Nil) {
			misses = append(misses, articleID)
			continue
		}
		entries[articleID] = tagArticleCacheEntry{
			ID: articleID, Title: fields[0], Describe: fields[1],
			CreateTime: fields[2], UpdateTime: fields[3], ViewNum: int(view),
		}
	}
	return entries, misses, nil
}

func tagArticleStringFields(values []any) ([]string, bool) {
	if len(values) != len(tagArticleHashFields) {
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

func (t *tagService) loadTagArticleMisses(ctx context.Context,
	ids []string,
) (map[string]tagArticleCacheEntry, error) {
	entries := make(map[string]tagArticleCacheEntry, len(ids))
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
	articles, err := t.articleModel.GetArticleListByIDList(ctx, numericIDs)
	if err != nil {
		return nil, err
	}

	pipe := t.redis.Pipeline()
	viewKey := cachekey.ArticleViewZSet().String()
	viewCmds := make(map[string]*redis.FloatCmd, len(articles))
	for _, item := range articles {
		if item == nil {
			continue
		}
		articleID := strconv.FormatUint(item.ID, 10)
		entries[articleID] = tagArticleCacheEntry{
			ID: articleID, Title: item.Title, Describe: item.Describe,
			CreateTime: item.CreateTime.Format(constants.TimeLayoutToSecond),
			UpdateTime: item.UpdateTime.Format(constants.TimeLayoutToSecond),
			ViewNum:    int(item.ViewNum),
		}
		pipe.HSet(ctx, cachekey.ArticleHash(articleID).String(), tagArticleCacheMap(item))
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

func tagArticleCacheMap(item *articleModel.Article) map[string]any {
	return map[string]any{
		"id":         item.ID,
		"title":      item.Title,
		"describe":   item.Describe,
		"content":    item.Content,
		"createTime": item.CreateTime.Format(constants.TimeLayoutToSecond),
		"updateTime": item.UpdateTime.Format(constants.TimeLayoutToSecond),
		"tagID":      articleTagIDValue(item.TagID),
		"tagName":    item.Tag.Name,
	}
}

func formatTagArticleTime(value, layout string) string {
	if len(value) <= len(layout) {
		return value
	}
	return value[:len(layout)]
}

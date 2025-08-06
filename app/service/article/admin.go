package article

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"meta-api/app/model/article"
	"meta-api/app/model/tag"
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
			if articleModel, err = a.articleModel.GetArticleDetailByID(ctx, id); err != nil {
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

	response := &types.AdminGetArticleDetailResponse{}
	hashKey := "article:" + request.ID + ":Hash"
	if exist := a.redis.Exists(ctx, hashKey); exist.Val() == 1 {
		// redis当中存在该数据
		fields := []string{"id", "title", "tagName", "describe", "content"}
		result, err := a.redis.HMGet(ctx, hashKey, fields...).Result()
		if err != nil {
			a.logger.Error("hmget error", zap.Error(err))
			return response, err
		}
		response.ID = result[0].(string)
		response.Title = result[1].(string)
		response.Tag = result[2].(string)
		response.Describe = result[3].(string)
		response.Content = result[4].(string)
	} else {
		// redis当中不存在该数据，从数据库当中获取数据
		id, err := strconv.ParseUint(request.ID, 10, 64)
		if err != nil {
			a.logger.Error("parse uint64 error", zap.Error(err))
			return response, err
		}
		articleInfo, err := a.articleModel.GetArticleDetailByID(ctx, id)
		if err != nil || articleInfo.ID == 0 {
			a.logger.Error("get article detail by id error", zap.Error(err))
			return response, err
		}

		// 缓存文章信息
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
		if err = a.redis.HMSet(ctx, "article:"+response.ID+":Hash", mapData).Err(); err != nil {
			return response, err
		}

		response.ID = strconv.FormatUint(articleInfo.ID, 10)
		response.Title = articleInfo.Title
		response.Tag = articleInfo.TagName
		response.Describe = articleInfo.Describe
		response.Content = articleInfo.Content
	}

	return response, nil
}

// AdminAddArticle 添加文章
func (a *articleService) AdminAddArticle(ctx context.Context, request *types.AdminAddArticleRequest) error {

	// 获取tag
	tagInfo, err := a.tagModel.FindTagByName(ctx, request.Tag)
	if err != nil {
		a.logger.Error("failed to find tag", zap.Error(err))
		return fmt.Errorf("failed to find tag, error: %w", err)
	}
	if tagInfo == nil || tagInfo.ID == 0 {
		tagID, err := a.idGenerator.NextID()
		if err != nil {
			a.logger.Error("generate tag id error", zap.Error(err))
			return fmt.Errorf("generate tag id error: %w", err)
		}
		tagInfo = &tag.Tag{
			ID:   tagID,
			Name: request.Tag,
		}
		if err = a.tagModel.CreateTag(ctx, tagInfo); err != nil {
			a.logger.Error("failed to create tag", zap.Error(err))
			return fmt.Errorf("failed to create tag: %w", err)
		}
	}

	// 创建文章
	articleID, err := a.idGenerator.NextID()
	if err != nil {
		a.logger.Error("generate article id error", zap.Error(err))
		return fmt.Errorf("generate article id error: %w", err)
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		a.logger.Error("failed to load location", zap.Error(err))
		return fmt.Errorf("failed to load location, error: %w", err)
	}
	articleInfo := &article.Article{
		ID:         articleID,
		Title:      request.Title,
		Describe:   request.Describe,
		Content:    request.Content,
		ViewNum:    0,
		CreateTime: time.Now().In(loc),
		UpdateTime: time.Now().In(loc),
		TagID:      tagInfo.ID,
	}
	if err = a.articleModel.CreateArticle(ctx, articleInfo); err != nil {
		a.logger.Error("failed to create article", zap.Error(err))
		return fmt.Errorf("failed to create article, error: %w", err)
	}

	// 有序集合：按时间排序
	timestamp := articleInfo.CreateTime.UnixNano() / int64(time.Millisecond)
	timeMember := []redis.Z{
		{Score: float64(timestamp), Member: articleInfo.ID},
	}
	if err = a.redis.ZAdd(ctx, "article:time:ZSet", timeMember...).Err(); err != nil {
		return err
	}

	// 有序集合：按浏览量排序
	viewMember := []redis.Z{
		{Score: float64(articleInfo.ViewNum), Member: articleInfo.ID},
	}
	if err = a.redis.ZAdd(ctx, "article:view:ZSet", viewMember...).Err(); err != nil {
		return err
	}

	// 有序集合：按标签对应的文章数量排序
	tagArticleNumKey := "tag:articleNum:ZSet"
	tagName := tagInfo.Name
	err = a.redis.ZScore(ctx, tagArticleNumKey, tagName).Err()
	switch {
	case errors.Is(err, redis.Nil):
		if err = a.redis.ZAdd(ctx, tagArticleNumKey, redis.Z{Score: 1, Member: tagName}).Err(); err != nil {
			a.logger.Error("failed to add tagIDArticleKey", zap.Error(err))
			return err
		}
	case err != nil:
		a.logger.Error("failed to query tagIDArticleKey", zap.Error(err))
		return err
	default:
		if err = a.redis.ZIncrBy(ctx, tagArticleNumKey, 1, tagName).Err(); err != nil {
			a.logger.Error("failed to add tagIDArticleKey", zap.Error(err))
			return err
		}
	}

	// 有序集合：按标签下的文章的创建时间排序
	timeMember = []redis.Z{
		{Score: float64(timestamp), Member: articleInfo.ID},
	}
	if err = a.redis.ZAdd(ctx, tagName+":article"+":ZSet", timeMember...).Err(); err != nil {
		a.logger.Error("failed to add tagIDArticleKey", zap.Error(err))
		return err
	}

	return nil
}

// AdminUpdateArticle 更新文章
func (a *articleService) AdminUpdateArticle(ctx context.Context, request *types.AdminUpdateArticleRequest) error {

	// 处理Tag
	tagInfo, err := a.tagModel.FindTagByName(ctx, request.Tag)
	if err != nil {
		a.logger.Error("failed to find tag", zap.Error(err))
		return fmt.Errorf("failed to find tag, error: %w", err)
	}
	if tagInfo == nil || tagInfo.ID == 0 {
		tagID, err := a.idGenerator.NextID()
		if err != nil {
			a.logger.Error("generate tag id error", zap.Error(err))
			return fmt.Errorf("generate tag id error: %w", err)
		}
		tagInfo = &tag.Tag{
			ID:   tagID,
			Name: request.Tag,
		}
		if err = a.tagModel.CreateTag(ctx, tagInfo); err != nil {
			a.logger.Error("failed to create tag", zap.Error(err))
			return fmt.Errorf("failed to create tag: %w", err)
		}
	}

	// 需要获取当前文章的浏览量，避免浏览量丢失
	viewNum, err := a.redis.ZScore(ctx, "article:view:ZSet", request.ID).Result()
	if err != nil {
		a.logger.Error("failed to query article:view:ZSet", zap.Error(err))
		return fmt.Errorf("failed to query article:view:ZSet: %w", err)
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		a.logger.Error("failed to load location", zap.Error(err))
		return fmt.Errorf("failed to load location: %w", err)
	}

	// 更新文章
	id, err := strconv.ParseUint(request.ID, 10, 64)
	if err != nil {
		a.logger.Error("parse uint64 error", zap.Error(err))
		return err
	}
	articleInfo := &article.Article{
		ID:         id,
		Title:      request.Title,
		Describe:   request.Describe,
		Content:    request.Content,
		ViewNum:    uint64(viewNum),
		UpdateTime: time.Now().In(loc),
		TagID:      tagInfo.ID,
	}
	if err = a.articleModel.UpdateArticle(ctx, articleInfo); err != nil {
		a.logger.Error("failed to update article", zap.Error(err))
		return fmt.Errorf("failed to update article: %w", err)
	}

	// 处理缓存数据
	if err = a.redis.Del(ctx, "article:"+request.ID+":Hash").Err(); err != nil {
		a.logger.Error("failed to delete hash", zap.Error(err))
		return fmt.Errorf("failed to delete hash: %w", err)
	}
	if err = a.redis.Del(ctx, "tag:articleNum:ZSet").Err(); err != nil {
		a.logger.Error("failed to delete tag:articleNum:ZSet", zap.Error(err))
		return fmt.Errorf("failed to delete tag:articleNum:ZSet: %w", err)
	}
	key := strconv.Itoa(int(tagInfo.ID)) + ":article" + ":ZSet"
	if err = a.redis.Del(ctx, key).Err(); err != nil {
		a.logger.Error("failed to delete tagID:article:ZSet", zap.Error(err))
		return fmt.Errorf("failed to delete tagID:article:ZSet: %w", err)
	}

	return nil
}

// AdminDeleteArticle 删除文章
func (a *articleService) AdminDeleteArticle(ctx context.Context, request *types.AdminDeleteArticleRequest) error {
	articleID := request.ID
	id, err := strconv.ParseUint(request.ID, 10, 64)
	if err != nil {
		a.logger.Error("parse uint64 error", zap.Error(err))
		return err
	}
	tagName, err := a.articleModel.DelArticleAndReturnTagName(ctx, id)
	if err != nil {
		a.logger.Error("failed to delete article", zap.Error(err))
		return fmt.Errorf("failed to delete article: %w", err)
	}

	// 删除文章的hash
	if err = a.redis.Del(ctx, "article:"+articleID+":Hash").Err(); err != nil {
		a.logger.Error("failed to delete hash", zap.Error(err))
		return err
	}

	// 删除article:time:ZSet里面的成员
	if err = a.redis.ZRem(ctx, "article:time:ZSet", articleID).Err(); err != nil {
		a.logger.Error("failed to delete article:time:ZSet", zap.Error(err))
		return err
	}

	// 删除article:view:ZSet里面的成员
	if err = a.redis.ZRem(ctx, "article:view:ZSet", articleID).Err(); err != nil {
		a.logger.Error("failed to delete article:view:ZSet", zap.Error(err))
		return err
	}

	// 删除tag:articleNum:ZSet整个有序集合
	if err = a.redis.Del(ctx, "tag:articleNum:ZSet").Err(); err != nil {
		a.logger.Error("failed to delete tag:articleNum:ZSet", zap.Error(err))
		return err
	}

	// 删除tagID:article:ZSet整个有序集合
	tagNameArticleKey := tagName + ":article" + ":ZSet"
	if err = a.redis.Del(ctx, tagNameArticleKey).Err(); err != nil {
		a.logger.Error("failed to delete tagIDArticleKey", zap.Error(err))
		return err
	}

	return nil
}

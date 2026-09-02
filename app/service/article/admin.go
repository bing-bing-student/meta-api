package article

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"meta-api/app/model/article"
	"meta-api/app/model/tag"
	"meta-api/common/cachekey"
	"meta-api/common/constants"
	"meta-api/common/idutil"
	"meta-api/common/types"
	"meta-api/pkg/cos"
)

var dangerousSVGPattern = regexp.MustCompile(`(?is)<\s*(script|iframe|object|embed|link|meta)\b|\bon[a-z]+\s*=|javascript:`)

type articleImageType struct {
	mime string
	ext  string
}

var allowedArticleImageTypes = map[string]articleImageType{
	"image/png":     {mime: "image/png", ext: ".png"},
	"image/jpeg":    {mime: "image/jpeg", ext: ".jpg"},
	"image/webp":    {mime: "image/webp", ext: ".webp"},
	"image/gif":     {mime: "image/gif", ext: ".gif"},
	"image/svg+xml": {mime: "image/svg+xml", ext: ".svg"},
}

// AdminGetArticleList 管理员获取文章列表
func (a *articleService) AdminGetArticleList(ctx context.Context,
	request *types.AdminGetArticleListRequest) (*types.AdminGetArticleListResponse, error) {

	response := &types.AdminGetArticleListResponse{}
	start := (request.Page - 1) * request.PageSize
	stop := start + request.PageSize - 1

	zSetKey, ok := cachekey.ArticleOrderZSet(request.Order)
	if !ok {
		a.logger.Error("invalid article order", zap.String("order", request.Order))
		return response, fmt.Errorf("invalid article order: %s", request.Order)
	}
	articleIDZSet, total, err := a.readArticlePage(ctx, zSetKey.String(), start, stop)
	if err != nil {
		a.logger.Error("failed to get article:time/view:ZSet", zap.Error(err))
		return response, err
	}
	articleIDs := make([]string, 0, len(articleIDZSet))
	for _, z := range articleIDZSet {
		articleID, memberOK := z.Member.(string)
		if !memberOK {
			return response, fmt.Errorf("invalid article cache member type %T", z.Member)
		}
		articleIDs = append(articleIDs, articleID)
	}
	entries, misses, err := a.readArticleListCache(ctx, articleIDs)
	if err != nil {
		return response, err
	}
	loaded, err := a.loadArticleListMisses(ctx, misses)
	if err != nil {
		return response, err
	}
	for id, entry := range loaded {
		entries[id] = entry
	}

	response.Rows = make([]types.AdminGetArticleListItem, 0, len(articleIDs))
	for _, articleID := range articleIDs {
		entry, exists := entries[articleID]
		if !exists {
			return response, fmt.Errorf("article %s not found", articleID)
		}
		response.Rows = append(response.Rows, types.AdminGetArticleListItem{
			ID:         articleID,
			Title:      entry.Title,
			Tag:        entry.TagName,
			ViewNum:    entry.ViewNum,
			CreateTime: formatCachedArticleTime(entry.CreateTime, constants.TimeLayoutToMinute),
			UpdateTime: formatCachedArticleTime(entry.UpdateTime, constants.TimeLayoutToMinute),
		})
	}
	response.Total = int(total)
	return response, nil
}

// AdminGetArticleDetail 获取文章详情
func (a *articleService) AdminGetArticleDetail(ctx context.Context,
	request *types.AdminGetArticleDetailRequest) (*types.AdminGetArticleDetailResponse, error) {

	response := &types.AdminGetArticleDetailResponse{}
	hashKey := cachekey.ArticleHash(request.ID).String()
	fields := []string{"id", "title", "tagName", "describe", "content"}
	result, err := a.redis.HMGet(ctx, hashKey, fields...).Result()
	if err != nil {
		a.logger.Error("HMGET error", zap.Error(err))
		return response, err
	}
	cached, cacheHit := redisStringFields(result, len(fields))
	if cacheHit {
		response.ID = cached[0]
		response.Title = cached[1]
		response.Tag = cached[2]
		response.Describe = cached[3]
		response.Content = cached[4]
	} else {
		// redis当中不存在该数据，从数据库当中获取数据
		id, err := idutil.ParseID("articleID", request.ID)
		if err != nil {
			a.logger.Error("invalid article id", zap.Error(err))
			return response, err
		}
		articleInfo, err := a.articleModel.GetArticleDetailByID(ctx, id)
		if err != nil || articleInfo.ID == 0 {
			a.logger.Error("get article detail by id error", zap.Error(err))
			return response, err
		}

		// 缓存文章信息
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
func (a *articleService) AdminAddArticle(ctx context.Context,
	request *types.AdminAddArticleRequest) (*types.AdminSaveArticleResponse, error) {

	// 获取 tag
	tagInfo, err := a.tagModel.FindTagByName(ctx, request.Tag)
	if err != nil {
		a.logger.Error("failed to find tag", zap.Error(err))
		return nil, fmt.Errorf("failed to find tag, error: %w", err)
	}
	if tagInfo == nil || tagInfo.ID == 0 {
		tagID, err := a.idGenerator.NextID()
		if err != nil {
			a.logger.Error("generate tag id error", zap.Error(err))
			return nil, fmt.Errorf("generate tag id error: %w", err)
		}
		tagInfo = &tag.Tag{
			ID:   tagID,
			Name: request.Tag,
		}
		if err = a.tagModel.CreateTag(ctx, tagInfo); err != nil {
			a.logger.Error("failed to create tag", zap.Error(err))
			return nil, fmt.Errorf("failed to create tag: %w", err)
		}
	}

	// 创建文章
	articleID, err := a.idGenerator.NextID()
	if err != nil {
		a.logger.Error("generate article id error", zap.Error(err))
		return nil, fmt.Errorf("generate article id error: %w", err)
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		a.logger.Error("failed to load location", zap.Error(err))
		return nil, fmt.Errorf("failed to load location, error: %w", err)
	}
	now := time.Now().In(loc)
	articleInfo := &article.Article{
		ID:            articleID,
		Title:         request.Title,
		Describe:      request.Describe,
		Content:       request.Content,
		ViewNum:       0,
		Status:        article.ArticleStatusPublished,
		PublishedTime: &now,
		CreateTime:    now,
		UpdateTime:    now,
		TagID:         &tagInfo.ID,
	}
	if err = a.articleModel.CreateArticle(ctx, articleInfo); err != nil {
		a.logger.Error("failed to create article", zap.Error(err))
		return nil, fmt.Errorf("failed to create article, error: %w", err)
	}
	if err = a.syncPublishedArticleImageReferences(ctx, articleInfo.ID, articleInfo.Content); err != nil {
		a.logger.Error("failed to sync article image references", zap.Error(err))
		return nil, fmt.Errorf("failed to sync article image references: %w", err)
	}

	// 有序集合：按时间排序
	timeMember := []redis.Z{
		{Score: cachekey.ArticleTimeScore(articleInfo.CreateTime), Member: articleInfo.ID},
	}
	if err = a.redis.ZAdd(ctx, cachekey.ArticleTimeZSet().String(), timeMember...).Err(); err != nil {
		return nil, err
	}

	// 有序集合：按浏览量排序
	viewMember := []redis.Z{
		{Score: cachekey.ArticleViewScore(articleInfo.ViewNum), Member: articleInfo.ID},
	}
	if err = a.redis.ZAdd(ctx, cachekey.ArticleViewZSet().String(), viewMember...).Err(); err != nil {
		return nil, err
	}

	// 有序集合：按标签对应的文章数量排序
	tagArticleNumKey := cachekey.TagArticleNumZSet().String()
	tagName := tagInfo.Name
	err = a.redis.ZScore(ctx, tagArticleNumKey, tagName).Err()
	switch {
	case errors.Is(err, redis.Nil):
		if err = a.redis.ZAdd(ctx, tagArticleNumKey, redis.Z{Score: 1, Member: tagName}).Err(); err != nil {
			a.logger.Error("failed to add tagIDArticleKey", zap.Error(err))
			return nil, err
		}
	case err != nil:
		a.logger.Error("failed to query tagIDArticleKey", zap.Error(err))
		return nil, err
	default:
		if err = a.redis.ZIncrBy(ctx, tagArticleNumKey, 1, tagName).Err(); err != nil {
			a.logger.Error("failed to add tagIDArticleKey", zap.Error(err))
			return nil, err
		}
	}

	// 有序集合：按标签下的文章的创建时间排序
	timeMember = []redis.Z{
		{Score: cachekey.ArticleTimeScore(articleInfo.CreateTime), Member: articleInfo.ID},
	}
	if err = a.redis.ZAdd(ctx, cachekey.TagArticleListZSet(tagName).String(), timeMember...).Err(); err != nil {
		a.logger.Error("failed to add tagIDArticleKey", zap.Error(err))
		return nil, err
	}

	articleIDString := strconv.FormatUint(articleID, 10)

	// 刷新 sitemap 内部缓存，让新增文章 URL 尽快出现在 sitemap.xml。
	a.sitemap.RefreshArticles(articleIDString)

	return &types.AdminSaveArticleResponse{ID: articleIDString}, nil
}

// AdminUpdateArticle 更新文章
func (a *articleService) AdminUpdateArticle(ctx context.Context,
	request *types.AdminUpdateArticleRequest) (*types.AdminSaveArticleResponse, error) {

	// 解析文章 ID
	id, err := idutil.ParseID("articleID", request.ID)
	if err != nil {
		a.logger.Error("invalid article id", zap.Error(err))
		return nil, err
	}

	// 在更新之前先查出旧文章信息，主要是为了拿到旧 tagName
	oldArticle, err := a.articleModel.GetArticleDetailByID(ctx, id)
	if err != nil {
		a.logger.Error("failed to get old article info", zap.Error(err))
		return nil, fmt.Errorf("failed to get old article info: %w", err)
	}
	oldTagName := oldArticle.TagName

	// 处理 Tag
	tagInfo, err := a.tagModel.FindTagByName(ctx, request.Tag)
	if err != nil {
		a.logger.Error("failed to find tag", zap.Error(err))
		return nil, fmt.Errorf("failed to find tag, error: %w", err)
	}
	if tagInfo == nil || tagInfo.ID == 0 {
		tagID, err := a.idGenerator.NextID()
		if err != nil {
			a.logger.Error("generate tag id error", zap.Error(err))
			return nil, fmt.Errorf("generate tag id error: %w", err)
		}
		tagInfo = &tag.Tag{
			ID:   tagID,
			Name: request.Tag,
		}
		if err = a.tagModel.CreateTag(ctx, tagInfo); err != nil {
			a.logger.Error("failed to create tag", zap.Error(err))
			return nil, fmt.Errorf("failed to create tag: %w", err)
		}
	}
	newTagName := tagInfo.Name

	// 需要获取当前文章的浏览量，避免浏览量丢失
	viewNum, err := a.redis.ZScore(ctx, cachekey.ArticleViewZSet().String(), request.ID).Result()
	if err != nil {
		a.logger.Error("failed to query article:view:ZSet", zap.Error(err))
		return nil, fmt.Errorf("failed to query article:view:ZSet: %w", err)
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		a.logger.Error("failed to load location", zap.Error(err))
		return nil, fmt.Errorf("failed to load location: %w", err)
	}

	// 更新文章
	articleInfo := &article.Article{
		ID:         id,
		Title:      request.Title,
		Describe:   request.Describe,
		Content:    request.Content,
		ViewNum:    uint64(viewNum),
		UpdateTime: time.Now().In(loc),
		TagID:      &tagInfo.ID,
	}
	if err = a.articleModel.UpdateArticle(ctx, articleInfo); err != nil {
		a.logger.Error("failed to update article", zap.Error(err))
		return nil, fmt.Errorf("failed to update article: %w", err)
	}
	if err = a.syncPublishedArticleImageReferences(ctx, articleInfo.ID, articleInfo.Content); err != nil {
		a.logger.Error("failed to sync article image references", zap.Error(err))
		return nil, fmt.Errorf("failed to sync article image references: %w", err)
	}

	// 处理缓存数据
	if err = a.redis.Del(ctx, cachekey.ArticleHash(request.ID).String()).Err(); err != nil {
		a.logger.Error("failed to delete hash", zap.Error(err))
		return nil, fmt.Errorf("failed to delete hash: %w", err)
	}
	if err = a.redis.Del(ctx, cachekey.TagArticleNumZSet().String()).Err(); err != nil {
		a.logger.Error("failed to delete tag:articleNum:ZSet", zap.Error(err))
		return nil, fmt.Errorf("failed to delete tag:articleNum:ZSet: %w", err)
	}

	// 清理「标签下的文章列表」ZSet 缓存：
	if err = a.redis.Del(ctx, cachekey.TagArticleListZSet(oldTagName).String()).Err(); err != nil {
		a.logger.Error("failed to delete oldTagName:article:ZSet",
			zap.String("oldTagName", oldTagName), zap.Error(err))
		return nil, fmt.Errorf("failed to delete oldTagName:article:ZSet: %w", err)
	}
	if newTagName != oldTagName {
		if err = a.redis.Del(ctx, cachekey.TagArticleListZSet(newTagName).String()).Err(); err != nil {
			a.logger.Error("failed to delete newTagName:article:ZSet",
				zap.String("newTagName", newTagName), zap.Error(err))
			return nil, fmt.Errorf("failed to delete newTagName:article:ZSet: %w", err)
		}
	}

	// 刷新 sitemap 内部缓存，让文章 lastmod 或标签变更尽快反映到 sitemap.xml。
	a.sitemap.RefreshArticles(request.ID)

	// 清理 CDN 上 /article-detail/<id> 的文章详情 HTML 缓存。
	// 文章标题、正文、摘要或标签变化后，旧 HTML 命中边缘节点会继续展示旧内容。
	if err = a.cdn.PurgeArticles(request.ID); err != nil {
		a.logger.Error("failed to purge article CDN cache", zap.String("article_id", request.ID), zap.Error(err))
		return nil, fmt.Errorf("failed to purge article CDN cache: %w", err)
	}

	return &types.AdminSaveArticleResponse{ID: request.ID}, nil
}

// AdminDeleteArticle 删除文章
func (a *articleService) AdminDeleteArticle(ctx context.Context, request *types.AdminDeleteArticleRequest) error {
	articleID := request.ID
	id, err := idutil.ParseID("articleID", request.ID)
	if err != nil {
		a.logger.Error("invalid article id", zap.Error(err))
		return err
	}
	tagName, err := a.articleModel.GetArticleDeleteInfo(ctx, id)
	if err != nil {
		a.logger.Error("failed to get article delete info", zap.Error(err))
		return fmt.Errorf("failed to get article delete info: %w", err)
	}

	// 删除文章前先清理 CDN 上 /article-detail/<id> 的文章详情 HTML 缓存。
	// 若 CDN 清理失败，保留数据库记录，避免后台提示失败但文章已被删除。
	if err = a.cdn.PurgeArticles(articleID); err != nil {
		a.logger.Error("failed to purge article CDN cache", zap.String("article_id", articleID), zap.Error(err))
		return fmt.Errorf("failed to purge article CDN cache: %w", err)
	}

	if err = a.syncPublishedArticleImageReferences(ctx, id, ""); err != nil {
		a.logger.Error("failed to clear article image references", zap.Error(err))
		return fmt.Errorf("failed to clear article image references: %w", err)
	}

	if err = a.articleModel.DeleteArticleByID(ctx, id); err != nil {
		a.logger.Error("failed to delete article", zap.Error(err))
		return fmt.Errorf("failed to delete article: %w", err)
	}

	// 删除文章的 hash
	if err = a.redis.Del(ctx, cachekey.ArticleHash(articleID).String()).Err(); err != nil {
		a.logger.Error("failed to delete hash", zap.Error(err))
		return err
	}
	// 文章删除会级联删除评论，同步删除前台评论快照。
	if err = a.redis.Del(ctx, cachekey.CommentApprovedArticle(articleID).String()).Err(); err != nil {
		a.logger.Error("failed to delete approved comment cache", zap.Error(err))
	}

	// 删除article:time:ZSet里面的成员
	if err = a.redis.ZRem(ctx, cachekey.ArticleTimeZSet().String(), articleID).Err(); err != nil {
		a.logger.Error("failed to delete article:time:ZSet", zap.Error(err))
		return err
	}

	// 删除article:view:ZSet里面的成员
	if err = a.redis.ZRem(ctx, cachekey.ArticleViewZSet().String(), articleID).Err(); err != nil {
		a.logger.Error("failed to delete article:view:ZSet", zap.Error(err))
		return err
	}

	// 删除tag:articleNum:ZSet整个有序集合
	if err = a.redis.Del(ctx, cachekey.TagArticleNumZSet().String()).Err(); err != nil {
		a.logger.Error("failed to delete tag:articleNum:ZSet", zap.Error(err))
		return err
	}

	// 删除tagID:article:ZSet整个有序集合
	tagNameArticleKey := cachekey.TagArticleListZSet(tagName).String()
	if err = a.redis.Del(ctx, tagNameArticleKey).Err(); err != nil {
		a.logger.Error("failed to delete tagIDArticleKey", zap.Error(err))
		return err
	}

	// 刷新 sitemap 内部缓存，让被删文章 URL 尽快从 sitemap.xml 移除。
	a.sitemap.RefreshArticles(articleID)

	return nil
}

// AdminUploadArticleImage 上传文章图片
func (a *articleService) AdminUploadArticleImage(ctx context.Context, fileName string, contentType string,
	content []byte) (*types.AdminUploadArticleImageResponse, error) {

	if len(content) == 0 {
		return nil, fmt.Errorf("empty image file")
	}
	if int64(len(content)) > constants.MaxArticleImageSize {
		return nil, fmt.Errorf("image file too large")
	}

	imageType, err := detectArticleImageType(fileName, contentType, content)
	if err != nil {
		return nil, err
	}
	if imageType.mime == "image/svg+xml" && !isSafeArticleSVG(content) {
		return nil, fmt.Errorf("unsafe svg content")
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	now := time.Now()
	storedName := fmt.Sprintf("%s%03d%s", now.Format("20060102150405"), now.Nanosecond()/int(time.Millisecond), imageType.ext)
	objectName := storedName
	publicURL, err := a.imageStore.Upload(ctx, objectName, content, imageType.mime)
	if err != nil {
		if errors.Is(err, cos.ErrDisabled) {
			return nil, fmt.Errorf("article image storage is not configured: %w", err)
		}
		return nil, err
	}
	imageID, err := a.idGenerator.NextID()
	if err != nil {
		return nil, fmt.Errorf("generate article image id: %w", err)
	}
	if err = a.articleModel.CreateArticleImage(ctx, &article.ArticleImage{
		ID:         imageID,
		ObjectKey:  a.imageStore.ObjectKey(objectName),
		URL:        publicURL,
		ImageName:  storedName,
		Mime:       imageType.mime,
		Size:       int64(len(content)),
		Status:     article.ImageStatusUnused,
		CreateTime: now,
		UpdateTime: now,
	}); err != nil {
		return nil, err
	}

	return &types.AdminUploadArticleImageResponse{
		URL:       publicURL,
		ImageName: storedName,
		Size:      int64(len(content)),
		Mime:      imageType.mime,
	}, nil
}

// detectArticleImageType 检测文章图片类型
func detectArticleImageType(fileName string, contentType string, content []byte) (articleImageType, error) {
	normalizedContentType := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	if normalizedContentType == "image/jpg" {
		normalizedContentType = "image/jpeg"
	}

	ext := strings.ToLower(filepath.Ext(fileName))
	if ext == ".jpg" {
		ext = ".jpeg"
	}
	if ext == ".svg" && looksLikeSVG(content) {
		return allowedArticleImageTypes["image/svg+xml"], nil
	}

	detected := strings.ToLower(http.DetectContentType(content))
	if detected == "image/jpg" {
		detected = "image/jpeg"
	}
	if detectedType, ok := allowedArticleImageTypes[detected]; ok {
		return detectedType, nil
	}
	if requestType, ok := allowedArticleImageTypes[normalizedContentType]; ok && requestType.mime != "image/svg+xml" {
		return requestType, nil
	}
	if mimeByExt := mime.TypeByExtension(ext); mimeByExt != "" {
		mimeByExt = strings.ToLower(strings.Split(mimeByExt, ";")[0])
		if extType, ok := allowedArticleImageTypes[mimeByExt]; ok && extType.mime != "image/svg+xml" {
			return extType, nil
		}
	}
	return articleImageType{}, fmt.Errorf("unsupported image type")
}

// looksLikeSVG 检查内容是否像 SVG 图片
func looksLikeSVG(content []byte) bool {
	snippet := string(content)
	if len(snippet) > 1024 {
		snippet = snippet[:1024]
	}
	snippet = strings.TrimSpace(snippet)
	return strings.Contains(strings.ToLower(snippet), "<svg")
}

// isSafeArticleSVG 检查文章 SVG 是否安全
func isSafeArticleSVG(content []byte) bool {
	text := strings.ToLower(string(content))
	return strings.Contains(text, "<svg") && !dangerousSVGPattern.MatchString(text)
}

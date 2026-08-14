package article

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"meta-api/app/model/article"
	"meta-api/app/model/tag"
	"meta-api/common/cachekey"
	"meta-api/common/constants"
	"meta-api/common/idutil"
	"meta-api/common/types"
)

func (a *articleService) AdminGetArticleDraftList(ctx context.Context,
	request *types.AdminGetArticleDraftListRequest) (*types.AdminGetArticleDraftListResponse, error) {
	offset := (request.Page - 1) * request.PageSize
	records, total, err := a.articleModel.ListArticleDrafts(ctx, offset, request.PageSize)
	if err != nil {
		return nil, err
	}

	rows := make([]types.AdminArticleDraftListItem, 0, len(records))
	for _, record := range records {
		draftType := "new"
		if record.PublishedID != nil {
			draftType = "edit"
		}
		item := types.AdminArticleDraftListItem{
			ID:         strconv.FormatUint(record.ID, 10),
			DraftType:  draftType,
			Title:      record.Title,
			Tag:        record.TagName,
			CreateTime: record.CreateTime.Format(constants.TimeLayoutToMinute),
			UpdateTime: record.UpdateTime.Format(constants.TimeLayoutToMinute),
		}
		if record.PublishedID != nil {
			item.ArticleID = strconv.FormatUint(*record.PublishedID, 10)
		}
		rows = append(rows, item)
	}
	return &types.AdminGetArticleDraftListResponse{
		Rows:  rows,
		Total: int(total),
	}, nil
}

func (a *articleService) AdminGetArticleDraftDetail(ctx context.Context,
	request *types.AdminGetArticleDraftDetailRequest) (*types.AdminGetArticleDraftDetailResponse, error) {
	id, err := idutil.ParseID("articleDraftID", request.ID)
	if err != nil {
		return nil, err
	}
	draft, err := a.articleModel.GetArticleDraftDetailByID(ctx, id)
	if err != nil {
		return nil, err
	}

	response := &types.AdminGetArticleDraftDetailResponse{
		ID:       strconv.FormatUint(draft.ID, 10),
		Title:    draft.Title,
		Tag:      draft.TagName,
		Describe: draft.Describe,
		Content:  draft.Content,
	}
	if draft.PublishedID != nil {
		response.ArticleID = strconv.FormatUint(*draft.PublishedID, 10)
	}
	return response, nil
}

func (a *articleService) AdminSaveArticleDraft(ctx context.Context,
	request *types.AdminSaveArticleDraftRequest) (*types.AdminSaveArticleResponse, error) {
	draftID, err := parseOptionalID("articleDraftID", request.ID)
	if err != nil {
		return nil, err
	}
	publishedIDValue, err := parseOptionalID("articleID", request.ArticleID)
	if err != nil {
		return nil, err
	}
	var publishedID *uint64
	if publishedIDValue != 0 {
		if _, err = a.articleModel.GetArticleDetailByID(ctx, publishedIDValue); err != nil {
			return nil, err
		}
		publishedID = &publishedIDValue
	}

	var tagID *uint64
	tagName := strings.TrimSpace(request.Tag)
	if tagName != "" {
		tagInfo, err := a.ensureTag(ctx, tagName)
		if err != nil {
			return nil, err
		}
		tagID = &tagInfo.ID
	}

	now := articleNow()
	title := strings.TrimSpace(request.Title)
	if draftID == 0 && publishedID != nil {
		if existing, err := a.articleModel.FindArticleDraftByPublishedID(ctx, *publishedID); err == nil {
			draftID = existing.ID
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}

	if draftID == 0 {
		if title == "" {
			title, err = a.nextDraftTitle(ctx)
			if err != nil {
				return nil, err
			}
		}
		newID, err := a.idGenerator.NextID()
		if err != nil {
			return nil, fmt.Errorf("generate article draft id: %w", err)
		}
		draft := &article.Article{
			ID:          newID,
			Title:       title,
			Describe:    strings.TrimSpace(request.Describe),
			Content:     request.Content,
			ViewNum:     0,
			Status:      article.ArticleStatusDraft,
			PublishedID: publishedID,
			CreateTime:  now,
			UpdateTime:  now,
			TagID:       tagID,
		}
		if err = a.articleModel.CreateArticleDraft(ctx, draft); err != nil {
			return nil, err
		}
		return &types.AdminSaveArticleResponse{
			ID:    strconv.FormatUint(newID, 10),
			Title: title,
		}, nil
	}

	existingDraft, err := a.articleModel.GetArticleDraftByID(ctx, draftID)
	if err != nil {
		return nil, err
	}
	if publishedID == nil {
		publishedID = existingDraft.PublishedID
	}
	if title == "" {
		title = existingDraft.Title
	}
	if strings.TrimSpace(title) == "" {
		title, err = a.nextDraftTitle(ctx)
		if err != nil {
			return nil, err
		}
	}
	draft := &article.Article{
		ID:          draftID,
		Title:       title,
		Describe:    strings.TrimSpace(request.Describe),
		Content:     request.Content,
		PublishedID: publishedID,
		UpdateTime:  now,
		TagID:       tagID,
	}
	if err = a.articleModel.UpdateArticleDraft(ctx, draft); err != nil {
		return nil, err
	}
	return &types.AdminSaveArticleResponse{
		ID:    strconv.FormatUint(draftID, 10),
		Title: title,
	}, nil
}

func (a *articleService) AdminPublishArticleDraft(ctx context.Context,
	request *types.AdminPublishArticleDraftRequest) (*types.AdminSaveArticleResponse, error) {
	draftID, err := idutil.ParseID("articleDraftID", request.ID)
	if err != nil {
		return nil, err
	}
	draft, err := a.articleModel.GetArticleDraftByID(ctx, draftID)
	if err != nil {
		return nil, err
	}

	tagInfo, err := a.ensureTag(ctx, strings.TrimSpace(request.Tag))
	if err != nil {
		return nil, err
	}
	now := articleNow()
	tagID := tagInfo.ID

	if draft.PublishedID == nil {
		published := &article.Article{
			ID:            draft.ID,
			Title:         strings.TrimSpace(request.Title),
			Describe:      strings.TrimSpace(request.Describe),
			Content:       request.Content,
			ViewNum:       0,
			Status:        article.ArticleStatusPublished,
			PublishedTime: &now,
			CreateTime:    now,
			UpdateTime:    now,
			TagID:         &tagID,
		}
		if err = a.articleModel.PublishNewArticleDraft(ctx, published); err != nil {
			return nil, err
		}
		if err = a.syncPublishedArticleImageReferences(ctx, published.ID, published.Content); err != nil {
			return nil, fmt.Errorf("failed to sync article image references: %w", err)
		}
		if err = a.addPublishedArticleCache(ctx, published, tagInfo.Name); err != nil {
			return nil, err
		}
		articleID := strconv.FormatUint(published.ID, 10)
		a.sitemap.RefreshArticles(articleID)
		return &types.AdminSaveArticleResponse{ID: articleID}, nil
	}

	articleID := *draft.PublishedID
	oldArticle, err := a.articleModel.GetArticleDetailByID(ctx, articleID)
	if err != nil {
		return nil, err
	}
	viewNum, err := a.currentArticleViewNum(ctx, strconv.FormatUint(articleID, 10), oldArticle.ViewNum)
	if err != nil {
		return nil, err
	}
	published := &article.Article{
		ID:            articleID,
		Title:         strings.TrimSpace(request.Title),
		Describe:      strings.TrimSpace(request.Describe),
		Content:       request.Content,
		ViewNum:       uint64(viewNum),
		PublishedTime: &now,
		UpdateTime:    now,
		TagID:         &tagID,
	}
	if err = a.articleModel.PublishArticleDraftToPublished(ctx, draftID, published); err != nil {
		return nil, err
	}
	if err = a.syncPublishedArticleImageReferences(ctx, articleID, published.Content); err != nil {
		return nil, fmt.Errorf("failed to sync article image references: %w", err)
	}
	articleIDString := strconv.FormatUint(articleID, 10)
	if err = a.invalidateUpdatedArticleCache(ctx, articleIDString, oldArticle.TagName, tagInfo.Name); err != nil {
		return nil, err
	}
	a.sitemap.RefreshArticles(articleIDString)
	if err = a.cdn.PurgeArticles(articleIDString); err != nil {
		a.logger.Error("failed to purge article CDN cache", zap.String("article_id", articleIDString), zap.Error(err))
		return nil, fmt.Errorf("failed to purge article CDN cache: %w", err)
	}
	return &types.AdminSaveArticleResponse{ID: articleIDString}, nil
}

func (a *articleService) AdminDeleteArticleDraft(ctx context.Context,
	request *types.AdminDeleteArticleDraftRequest) error {
	id, err := idutil.ParseID("articleDraftID", request.ID)
	if err != nil {
		return err
	}
	return a.articleModel.DeleteArticleDraftByID(ctx, id)
}

func (a *articleService) ensureTag(ctx context.Context, tagName string) (*tag.Tag, error) {
	if tagName == "" {
		return nil, fmt.Errorf("empty tag name")
	}
	tagInfo, err := a.tagModel.FindTagByName(ctx, tagName)
	if err != nil {
		a.logger.Error("failed to find tag", zap.Error(err))
		return nil, fmt.Errorf("failed to find tag, error: %w", err)
	}
	if tagInfo != nil && tagInfo.ID != 0 {
		return tagInfo, nil
	}

	tagID, err := a.idGenerator.NextID()
	if err != nil {
		a.logger.Error("generate tag id error", zap.Error(err))
		return nil, fmt.Errorf("generate tag id error: %w", err)
	}
	tagInfo = &tag.Tag{ID: tagID, Name: tagName}
	if err = a.tagModel.CreateTag(ctx, tagInfo); err != nil {
		a.logger.Error("failed to create tag", zap.Error(err))
		return nil, fmt.Errorf("failed to create tag: %w", err)
	}
	return tagInfo, nil
}

func (a *articleService) addPublishedArticleCache(ctx context.Context, articleInfo *article.Article, tagName string) error {
	timeMember := []redis.Z{
		{Score: cachekey.ArticleTimeScore(articleInfo.CreateTime), Member: articleInfo.ID},
	}
	if err := a.redis.ZAdd(ctx, cachekey.ArticleTimeZSet().String(), timeMember...).Err(); err != nil {
		return err
	}

	viewMember := []redis.Z{
		{Score: cachekey.ArticleViewScore(articleInfo.ViewNum), Member: articleInfo.ID},
	}
	if err := a.redis.ZAdd(ctx, cachekey.ArticleViewZSet().String(), viewMember...).Err(); err != nil {
		return err
	}

	tagArticleNumKey := cachekey.TagArticleNumZSet().String()
	err := a.redis.ZScore(ctx, tagArticleNumKey, tagName).Err()
	switch {
	case errors.Is(err, redis.Nil):
		if err = a.redis.ZAdd(ctx, tagArticleNumKey, redis.Z{Score: 1, Member: tagName}).Err(); err != nil {
			a.logger.Error("failed to add tag article count", zap.Error(err))
			return err
		}
	case err != nil:
		a.logger.Error("failed to query tag article count", zap.Error(err))
		return err
	default:
		if err = a.redis.ZIncrBy(ctx, tagArticleNumKey, 1, tagName).Err(); err != nil {
			a.logger.Error("failed to increase tag article count", zap.Error(err))
			return err
		}
	}

	if err = a.redis.ZAdd(ctx, cachekey.TagArticleListZSet(tagName).String(), timeMember...).Err(); err != nil {
		a.logger.Error("failed to add tag article list", zap.Error(err))
		return err
	}
	return nil
}

func (a *articleService) invalidateUpdatedArticleCache(ctx context.Context,
	articleID string, oldTagName string, newTagName string) error {
	if err := a.redis.Del(ctx, cachekey.ArticleHash(articleID).String()).Err(); err != nil {
		a.logger.Error("failed to delete hash", zap.Error(err))
		return fmt.Errorf("failed to delete hash: %w", err)
	}
	if err := a.redis.Del(ctx, cachekey.TagArticleNumZSet().String()).Err(); err != nil {
		a.logger.Error("failed to delete tag article count", zap.Error(err))
		return fmt.Errorf("failed to delete tag article count: %w", err)
	}
	if oldTagName != "" {
		if err := a.redis.Del(ctx, cachekey.TagArticleListZSet(oldTagName).String()).Err(); err != nil {
			a.logger.Error("failed to delete old tag article list",
				zap.String("oldTagName", oldTagName), zap.Error(err))
			return fmt.Errorf("failed to delete old tag article list: %w", err)
		}
	}
	if newTagName != "" && newTagName != oldTagName {
		if err := a.redis.Del(ctx, cachekey.TagArticleListZSet(newTagName).String()).Err(); err != nil {
			a.logger.Error("failed to delete new tag article list",
				zap.String("newTagName", newTagName), zap.Error(err))
			return fmt.Errorf("failed to delete new tag article list: %w", err)
		}
	}
	return nil
}

func (a *articleService) currentArticleViewNum(ctx context.Context, articleID string, fallback uint64) (float64, error) {
	viewNum, err := a.redis.ZScore(ctx, cachekey.ArticleViewZSet().String(), articleID).Result()
	if errors.Is(err, redis.Nil) {
		return float64(fallback), nil
	}
	if err != nil {
		a.logger.Error("failed to query article view", zap.Error(err))
		return 0, fmt.Errorf("failed to query article view: %w", err)
	}
	return viewNum, nil
}

func articleNow() time.Time {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.Now()
	}
	return time.Now().In(loc)
}

func articleTagIDValue(tagID *uint64) uint64 {
	if tagID == nil {
		return 0
	}
	return *tagID
}

func (a *articleService) nextDraftTitle(ctx context.Context) (string, error) {
	total, err := a.articleModel.CountArticleDrafts(ctx)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("草稿%d", total+1), nil
}

func parseOptionalID(name string, value string) (uint64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	id, err := idutil.ParseID(name, value)
	if err != nil {
		return 0, err
	}
	return id, nil
}

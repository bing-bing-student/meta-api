package article

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

type DraftListRecord struct {
	ID          uint64    `gorm:"column:id"`
	PublishedID *uint64   `gorm:"column:published_id"`
	Title       string    `gorm:"column:title"`
	TagName     string    `gorm:"column:tag_name"`
	CreateTime  time.Time `gorm:"column:create_time"`
	UpdateTime  time.Time `gorm:"column:update_time"`
}

func (a *articleModel) CreateArticleDraft(ctx context.Context, draft *Article) error {
	draft.Status = ArticleStatusDraft
	if err := a.mysql.WithContext(ctx).Model(&Article{}).Create(draft).Error; err != nil {
		return fmt.Errorf("failed to create article draft: %w", err)
	}
	return nil
}

func (a *articleModel) UpdateArticleDraft(ctx context.Context, draft *Article) error {
	values := articleDraftUpdateValues(draft)
	if err := a.mysql.WithContext(ctx).Model(&Article{}).
		Where("id = ? AND status = ?", draft.ID, ArticleStatusDraft).
		Updates(values).Error; err != nil {
		return fmt.Errorf("failed to update article draft: %w", err)
	}
	return nil
}

func (a *articleModel) GetArticleDraftByID(ctx context.Context, id uint64) (*Article, error) {
	draft := &Article{}
	if err := a.mysql.WithContext(ctx).Model(&Article{}).
		Where("id = ? AND status = ?", id, ArticleStatusDraft).
		First(draft).Error; err != nil {
		return nil, err
	}
	return draft, nil
}

func (a *articleModel) GetArticleDraftDetailByID(ctx context.Context, id uint64) (*Detail, error) {
	detail := &Detail{}
	if err := a.mysql.WithContext(ctx).Model(&Article{}).
		Table("article as a").
		Select("a.id, a.title, a.describe, a.content, a.view_num, a.status, a.published_id, a.published_time, a.create_time, a.update_time, a.tag_id, COALESCE(t.name, '') as tag_name").
		Joins("LEFT JOIN tag as t ON a.tag_id = t.id").
		Where("a.id = ? AND a.status = ?", id, ArticleStatusDraft).
		First(detail).Error; err != nil {
		return nil, err
	}
	return detail, nil
}

func (a *articleModel) FindArticleDraftByPublishedID(ctx context.Context, publishedID uint64) (*Article, error) {
	draft := &Article{}
	if err := a.mysql.WithContext(ctx).Model(&Article{}).
		Where("published_id = ? AND status = ?", publishedID, ArticleStatusDraft).
		First(draft).Error; err != nil {
		return nil, err
	}
	return draft, nil
}

func (a *articleModel) CountArticleDrafts(ctx context.Context) (int64, error) {
	var total int64
	if err := a.mysql.WithContext(ctx).Model(&Article{}).
		Where("status = ?", ArticleStatusDraft).
		Count(&total).Error; err != nil {
		return 0, fmt.Errorf("failed to count article drafts: %w", err)
	}
	return total, nil
}

func (a *articleModel) ListArticleDrafts(ctx context.Context, offset int, limit int) ([]DraftListRecord, int64, error) {
	var total int64
	if err := a.mysql.WithContext(ctx).Model(&Article{}).
		Where("status = ?", ArticleStatusDraft).
		Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count article drafts: %w", err)
	}
	if total == 0 {
		return []DraftListRecord{}, 0, nil
	}

	rows := make([]DraftListRecord, 0)
	if err := a.mysql.WithContext(ctx).Model(&Article{}).
		Table("article as a").
		Select("a.id, a.published_id, a.title, COALESCE(t.name, '') as tag_name, a.create_time, a.update_time").
		Joins("LEFT JOIN tag as t ON a.tag_id = t.id").
		Where("a.status = ?", ArticleStatusDraft).
		Order("a.update_time DESC, a.id DESC").
		Offset(offset).
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list article drafts: %w", err)
	}
	return rows, total, nil
}

func (a *articleModel) PublishNewArticleDraft(ctx context.Context, draft *Article) error {
	values := map[string]any{
		"title":          draft.Title,
		"describe":       draft.Describe,
		"content":        draft.Content,
		"view_num":       draft.ViewNum,
		"status":         ArticleStatusPublished,
		"published_id":   nil,
		"published_time": draft.PublishedTime,
		"tag_id":         draft.TagID,
		"create_time":    draft.CreateTime,
		"update_time":    draft.UpdateTime,
	}
	if err := a.mysql.WithContext(ctx).Model(&Article{}).
		Where("id = ? AND status = ?", draft.ID, ArticleStatusDraft).
		Updates(values).Error; err != nil {
		return fmt.Errorf("failed to publish new article draft: %w", err)
	}
	return nil
}

func (a *articleModel) PublishArticleDraftToPublished(ctx context.Context, draftID uint64,
	published *Article) error {
	values := map[string]any{
		"title":          published.Title,
		"describe":       published.Describe,
		"content":        published.Content,
		"view_num":       published.ViewNum,
		"tag_id":         published.TagID,
		"published_time": published.PublishedTime,
		"update_time":    published.UpdateTime,
	}
	return a.mysql.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Article{}).
			Where("id = ? AND status = ?", published.ID, ArticleStatusPublished).
			Updates(values).Error; err != nil {
			return fmt.Errorf("failed to update published article from draft: %w", err)
		}
		if err := tx.Where("id = ? AND status = ?", draftID, ArticleStatusDraft).
			Delete(&Article{}).Error; err != nil {
			return fmt.Errorf("failed to delete published article draft: %w", err)
		}
		return nil
	})
}

func (a *articleModel) DeleteArticleDraftByID(ctx context.Context, id uint64) error {
	if err := a.mysql.WithContext(ctx).
		Where("id = ? AND status = ?", id, ArticleStatusDraft).
		Delete(&Article{}).Error; err != nil {
		return fmt.Errorf("failed to delete article draft: %w", err)
	}
	return nil
}

func articleDraftUpdateValues(draft *Article) map[string]any {
	return map[string]any{
		"title":        draft.Title,
		"describe":     draft.Describe,
		"content":      draft.Content,
		"tag_id":       draft.TagID,
		"published_id": draft.PublishedID,
		"update_time":  draft.UpdateTime,
	}
}

package article

import (
	"context"
	"fmt"
	"time"

	"meta-api/app/model/tag"
)

type Article struct {
	ID         uint64    `gorm:"primary_key;NOT NULL"`
	Title      string    `gorm:"NOT NULL;unique"`
	Describe   string    `gorm:"NOT NULL"`
	Content    string    `gorm:"type:text;NOT NULL"`
	ViewNum    uint64    `gorm:"NOT NULL"`
	CreateTime time.Time `gorm:"NOT NULL"`
	UpdateTime time.Time `gorm:"NOT NULL"`
	TagID      uint64    `gorm:"NOT NULL"`
	Tag        tag.Tag   `gorm:"foreignKey:TagID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
}

type Detail struct {
	ID         uint64    `gorm:"column:id" json:"id"`
	Title      string    `gorm:"column:title" json:"title"`
	Describe   string    `gorm:"column:describe" json:"describe"`
	Content    string    `gorm:"column:content" json:"content"`
	ViewNum    uint64    `gorm:"column:view_num" json:"viewNum"`
	CreateTime time.Time `gorm:"column:create_time" json:"createTime"`
	UpdateTime time.Time `gorm:"column:update_time" json:"updateTime"`
	TagID      uint64    `gorm:"column:tag_id" json:"tagID"`
	TagName    string    `gorm:"column:tag_name" json:"tagName"`
}

type SearchArticle struct {
	ID       uint64 `gorm:"column:id" json:"id"`
	Title    string `gorm:"column:title" json:"title"`
	Describe string `gorm:"column:describe" json:"describe"`
	ViewNum  uint64 `gorm:"column:view_num" json:"viewNum"`
}

type DelArticle struct {
	ID      uint64 `gorm:"column:id" json:"id"`
	TagID   uint64 `gorm:"column:tag_id" json:"tagID"`
	TagName string `gorm:"column:tag_name" json:"tagName"`
}

type ListByTagName struct {
	ID         uint64    `gorm:"column:id" json:"ID"`
	CreateTime time.Time `gorm:"column:create_time" json:"createTime"`
}

type TimeAndViewZSet struct {
	ID         uint64    `gorm:"column:id" json:"ID"`
	ViewNum    uint64    `gorm:"column:view_num" json:"viewNum"`
	CreateTime time.Time `gorm:"column:create_time" json:"createTime"`
}

// CreateArticle 创建文章
func (a *articleModel) CreateArticle(ctx context.Context, newArticle *Article) error {
	if err := a.mysql.WithContext(ctx).Model(&Article{}).Create(newArticle).Error; err != nil {
		return fmt.Errorf("failed to create article: %w", err)
	}

	return nil
}

// UpdateArticle 更新文章
func (a *articleModel) UpdateArticle(ctx context.Context, articleInfo *Article) error {
	if err := a.mysql.WithContext(ctx).Model(&Article{}).
		Where("id = ?", articleInfo.ID).Updates(articleInfo).Error; err != nil {
		return fmt.Errorf("failed to update article: %w", err)
	}

	return nil
}

// UpdateArticleViewNum 更新文章浏览量
func (a *articleModel) UpdateArticleViewNum(ctx context.Context, id string, viewNum float64) error {
	if err := a.mysql.WithContext(ctx).Model(&Article{}).
		Where("id = ?", id).Update("view_num", viewNum).Error; err != nil {
		return fmt.Errorf("failed to update article: %w", err)
	}

	return nil
}

// GetArticleDetailByID 通过文章ID获取文章详情
func (a *articleModel) GetArticleDetailByID(ctx context.Context, id uint64) (*Detail, error) {
	detail := &Detail{}
	if err := a.mysql.WithContext(ctx).Model(&Article{}).
		Table("article as a").
		Select("a.id, a.title, a.describe, a.content, a.view_num, a.create_time, a.update_time, a.tag_id, b.name as tag_name").
		Joins("JOIN tag as b ON a.tag_id=b.id").
		Where("a.id = ?", id).
		First(detail).Error; err != nil {
		return nil, err
	}

	return detail, nil
}

// GetArticleListByTagName 通过标签名称获取文章信息
func (a *articleModel) GetArticleListByTagName(ctx context.Context, tagName string) ([]ListByTagName, error) {
	list := make([]ListByTagName, 0)
	if err := a.mysql.WithContext(ctx).Model(&Article{}).
		Select("article.id, article.create_time").
		Joins("JOIN tag ON tag.id = article.tag_id").
		Where("tag.name = ?", tagName).
		Find(&list).Error; err != nil {
		return nil, err
	}

	return list, nil
}

// DelArticleAndReturnTagName 删除文章并返回标签名
func (a *articleModel) DelArticleAndReturnTagName(ctx context.Context, id uint64) (string, error) {
	articleInfo := &DelArticle{}
	if err := a.mysql.WithContext(ctx).Model(&Article{}).Table("article as a").
		Select("a.id, a.tag_id, t.name as tag_name").
		Joins("LEFT JOIN tag as t ON a.tag_id = t.id").
		Where("a.id = ?", id).
		First(articleInfo).Error; err != nil {
		return "", err
	}

	if articleInfo.ID != 0 && articleInfo.TagName != "" {
		if err := a.mysql.WithContext(ctx).Model(&Article{}).Delete(&Article{}, id).Error; err != nil {
			return "", err
		}
		return articleInfo.TagName, nil
	}

	return "", fmt.Errorf("article not exist")
}

// SearchArticle 搜索文章
func (a *articleModel) SearchArticle(ctx context.Context, word string, limit, offset int) ([]SearchArticle, int64, error) {
	total := int64(0)
	list := make([]SearchArticle, 0)
	if err := a.mysql.WithContext(ctx).Model(&Article{}).
		Select("`id`, `title`, `describe`, `view_num`").
		Where("MATCH(content) AGAINST(? IN BOOLEAN MODE)", word+"*").
		Count(&total).
		Limit(limit).Offset(offset).
		Find(&list).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to query articles: %w", err)
	}
	return list, total, nil
}

// GetArticleListByIDList 通过id列表获取文章列表
func (a *articleModel) GetArticleListByIDList(ctx context.Context, ids []uint64) ([]*Article, error) {
	var articles []*Article
	err := a.mysql.WithContext(ctx).Model(&Article{}).Where("id IN ?", ids).Find(&articles).Error
	return articles, err
}

// UpdateArticleTagID 更新文章的tagID
func (a *articleModel) UpdateArticleTagID(ctx context.Context, articleIDList []string, tagID uint64) error {
	if err := a.mysql.WithContext(ctx).Model(&Article{}).
		Where("id IN ?", articleIDList).Update("tag_id", tagID).Error; err != nil {
		return err
	}
	return nil
}

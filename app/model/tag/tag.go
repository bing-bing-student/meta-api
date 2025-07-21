package tag

import (
	"context"
	"fmt"
	"time"
)

type Tag struct {
	ID   uint64 `gorm:"primary_key;NOT NULL"`
	Name string `gorm:"NOT NULL;unique;"`
}

// ArticleCountWithTag 标签下的文章数量
type ArticleCountWithTag struct {
	Name  string `gorm:"column:name"`
	Count int    `gorm:"column:count"`
}

// ArticleListByTagName 标签下的文章列表
type ArticleListByTagName struct {
	ID         uint64    `gorm:"column:id" json:"ID"`
	CreateTime time.Time `gorm:"column:create_time" json:"createTime"`
}

// CreateTag 创建标签
func (t *tagModel) CreateTag(ctx context.Context, newTag *Tag) error {
	if err := t.mysql.WithContext(ctx).Model(&Tag{}).Create(newTag).Error; err != nil {
		return fmt.Errorf("failed to create tag: %w", err)
	}

	return nil
}

// FindTagByName 查询标签是否存在
func (t *tagModel) FindTagByName(ctx context.Context, tagName string) (*Tag, error) {
	tagInfo := &Tag{}
	if err := t.mysql.WithContext(ctx).Model(&Tag{}).
		Where("name = ?", tagName).First(tagInfo).Error; err != nil {
		return nil, err
	}

	return tagInfo, nil
}

// GetArticleCountWithTagName 获取标签名称下的文章数量
func (t *tagModel) GetArticleCountWithTagName(ctx context.Context) ([]ArticleCountWithTag, error) {
	tagList := make([]ArticleCountWithTag, 0)
	if err := t.mysql.WithContext(ctx).Model(&Tag{}).Table("tag as t").
		Select("t.name, COUNT(a.id) AS count").
		Joins("JOIN article as a ON a.tag_id = t.id").
		Group("t.id").
		Having("COUNT(a.id) > 0").
		Order("count DESC").
		Find(&tagList).Error; err != nil {
		return nil, err
	}
	return tagList, nil
}

// GetArticleListByTagName 通过标签名获取文章列表
func (t *tagModel) GetArticleListByTagName(ctx context.Context, tagName string) ([]ArticleListByTagName, error) {
	articleList := make([]ArticleListByTagName, 0)
	if err := t.mysql.WithContext(ctx).Model(&Tag{}).Table("tag as t").
		Select("a.id, a.create_time").
		Joins("JOIN article as a ON a.tag_id = t.id").
		Where("t.name = ?", tagName).
		Find(&articleList).Error; err != nil {
		return nil, err
	}
	return articleList, nil
}

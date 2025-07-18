package tag

import (
	"context"
	"fmt"
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

package tag

import (
	"context"
	"fmt"
)

type Tag struct {
	ID   uint64 `gorm:"primary_key;NOT NULL"`
	Name string `gorm:"NOT NULL;unique;"`
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

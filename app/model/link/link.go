package link

import (
	"context"
	"fmt"
	"time"
)

type Link struct {
	ID         uint64    `gorm:"primary_key;NOT NULL"`
	Name       string    `gorm:"NOT NULL;unique"`
	URL        string    `gorm:"NOT NULL;unique"`
	CreateTime time.Time `gorm:"column:create_time" json:"createTime"`
	UpdateTime time.Time `gorm:"column:update_time" json:"updateTime"`
}

// GetLinkList 获取友链列表
func (l *linkModel) GetLinkList(ctx context.Context) ([]Link, error) {
	linkRows := make([]Link, 0)
	if err := l.mysql.WithContext(ctx).Model(&Link{}).Find(&linkRows).Error; err != nil {
		return nil, fmt.Errorf("failed to get linkList from mysql, err: %w", err)
	}
	return linkRows, nil
}

// CreateLink 添加友链
func (l *linkModel) CreateLink(ctx context.Context, newLink *Link) error {
	if err := l.mysql.WithContext(ctx).Model(&Link{}).Create(newLink).Error; err != nil {
		return fmt.Errorf("failed to add link, err: %w", err)
	}
	return nil
}

// UpdateLink 更新友链
func (l *linkModel) UpdateLink(ctx context.Context, linkInfo *Link) error {
	if err := l.mysql.WithContext(ctx).Model(&Link{}).Where("id = ?", linkInfo.ID).Updates(linkInfo).Error; err != nil {
		return fmt.Errorf("failed to update link, err: %w", err)
	}
	return nil
}

// DeleteLink 删除友链
func (l *linkModel) DeleteLink(ctx context.Context, id uint64) error {
	if err := l.mysql.WithContext(ctx).Model(&Link{}).Where("id = ?", id).Delete(&Link{}).Error; err != nil {
		return fmt.Errorf("failed to delete link, err: %w", err)
	}
	return nil
}

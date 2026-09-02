package sitedynamic

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

const (
	StatusPublished = "published"
	StatusHidden    = "hidden"
)

type SiteDynamic struct {
	ID        uint64 `gorm:"primary_key;NOT NULL"`
	Content   string `gorm:"column:content;type:varchar(50);NOT NULL"`
	Status    string `gorm:"column:status;type:varchar(20);NOT NULL;default:published;index:idx_site_dynamic_status_sort,priority:1"`
	SortOrder int    `gorm:"column:sort_order;NOT NULL;default:0;index:idx_site_dynamic_status_sort,priority:2;index:idx_site_dynamic_sort,priority:1"`
	// DeprecatedEventTime only keeps existing schemas writable. It is not part of
	// the site-dynamic domain, API contract, or ordering and can be removed by a
	// dedicated database migration after the stored values are no longer needed.
	DeprecatedEventTime time.Time `gorm:"column:event_time;NOT NULL;index:idx_site_dynamic_sort,priority:2"`
	CreateTime          time.Time `gorm:"column:create_time;NOT NULL"`
	UpdateTime          time.Time `gorm:"column:update_time;NOT NULL"`
}

func (m *siteDynamicModel) ListSiteDynamics(ctx context.Context) ([]SiteDynamic, error) {
	rows := make([]SiteDynamic, 0)
	if err := m.mysql.WithContext(ctx).Model(&SiteDynamic{}).
		Order("sort_order ASC").
		Order("id DESC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list site dynamics: %w", err)
	}
	return rows, nil
}

func (m *siteDynamicModel) ListPublishedSiteDynamics(ctx context.Context) ([]SiteDynamic, error) {
	rows := make([]SiteDynamic, 0)
	if err := m.mysql.WithContext(ctx).Model(&SiteDynamic{}).
		Where("status = ?", StatusPublished).
		Order("sort_order ASC").
		Order("id DESC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list published site dynamics: %w", err)
	}
	return rows, nil
}

func (m *siteDynamicModel) CreateSiteDynamic(ctx context.Context, item *SiteDynamic) error {
	return m.mysql.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var maxSortOrder int
		if err := tx.Model(&SiteDynamic{}).Select("COALESCE(MAX(sort_order), 0)").Scan(&maxSortOrder).Error; err != nil {
			return fmt.Errorf("get max site dynamic sort order: %w", err)
		}
		item.SortOrder = maxSortOrder + 1
		if err := tx.Model(&SiteDynamic{}).Create(item).Error; err != nil {
			return fmt.Errorf("create site dynamic: %w", err)
		}
		return nil
	})
}

func (m *siteDynamicModel) UpdateSiteDynamic(ctx context.Context, item *SiteDynamic) error {
	updates := map[string]any{
		"content":     item.Content,
		"status":      item.Status,
		"update_time": item.UpdateTime,
	}
	result := m.mysql.WithContext(ctx).Model(&SiteDynamic{}).Where("id = ?", item.ID).Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("update site dynamic: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		var count int64
		if err := m.mysql.WithContext(ctx).Model(&SiteDynamic{}).Where("id = ?", item.ID).Count(&count).Error; err != nil {
			return fmt.Errorf("confirm site dynamic exists: %w", err)
		}
		if count == 0 {
			return gorm.ErrRecordNotFound
		}
	}
	return nil
}

func (m *siteDynamicModel) ReorderSiteDynamics(ctx context.Context, ids []uint64, updateTime time.Time) error {
	return m.mysql.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for index, id := range ids {
			result := tx.Model(&SiteDynamic{}).
				Where("id = ?", id).
				Updates(map[string]any{
					"sort_order":  index + 1,
					"update_time": updateTime,
				})
			if result.Error != nil {
				return fmt.Errorf("reorder site dynamic: %w", result.Error)
			}
			if result.RowsAffected == 0 {
				return gorm.ErrRecordNotFound
			}
		}
		return nil
	})
}

func (m *siteDynamicModel) DeleteSiteDynamic(ctx context.Context, id uint64) error {
	result := m.mysql.WithContext(ctx).Model(&SiteDynamic{}).Where("id = ?", id).Delete(&SiteDynamic{})
	if result.Error != nil {
		return fmt.Errorf("delete site dynamic: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

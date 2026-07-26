package user

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

type User struct {
	ID                    uint64     `gorm:"primary_key;NOT NULL"`
	Provider              string     `gorm:"type:varchar(20);NOT NULL;uniqueIndex:idx_user_provider_uid,priority:1"`
	ProviderUserID        string     `gorm:"type:varchar(128);NOT NULL;uniqueIndex:idx_user_provider_uid,priority:2"`
	DisplayName           string     `gorm:"type:varchar(80);NOT NULL"`
	Handle                string     `gorm:"type:varchar(32);NOT NULL;uniqueIndex:idx_user_handle"`
	AvatarURL             string     `gorm:"type:varchar(500)"`
	ProfileURL            string     `gorm:"type:varchar(500)"`
	Email                 string     `gorm:"type:varchar(160)"`
	CommentDisabled       bool       `gorm:"column:comment_disabled;NOT NULL;default:false"`
	CommentDisabledReason string     `gorm:"column:comment_disabled_reason;type:varchar(200);NOT NULL;default:''"`
	CommentDisabledUntil  *time.Time `gorm:"column:comment_disabled_until"`
	SessionVersion        int64      `gorm:"column:session_version;NOT NULL;default:1"`
	CreateTime            time.Time  `gorm:"column:create_time;NOT NULL"`
	UpdateTime            time.Time  `gorm:"column:update_time;NOT NULL"`
}

type AdminListFilter struct {
	Handle            string
	DisplayName       string
	Provider          string
	CommentPermission string
	Now               time.Time
	Offset            int
	Limit             int
}

type AdminListItem struct {
	ID                    uint64
	Provider              string
	ProviderUserID        string
	DisplayName           string
	Handle                string
	AvatarURL             string
	ProfileURL            string
	Email                 string
	CommentDisabled       bool
	CommentDisabledReason string
	CommentDisabledUntil  *time.Time
	SessionVersion        int64
	CreateTime            time.Time
	UpdateTime            time.Time
	CommentCount          int64
	LastCommentTime       *time.Time
}

func (*User) TableName() string {
	return "user"
}

func (u *User) IsCommentDisabled(now time.Time) bool {
	if u == nil || !u.CommentDisabled {
		return false
	}
	if u.CommentDisabledUntil == nil {
		return true
	}
	return u.CommentDisabledUntil.After(now)
}

func (m *userModel) UpsertOAuthUser(ctx context.Context, user *User) (*User, error) {
	existing := &User{}
	err := m.mysql.WithContext(ctx).Model(&User{}).
		Where("provider = ? AND provider_user_id = ?", user.Provider, user.ProviderUserID).
		First(existing).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err = m.mysql.WithContext(ctx).Model(&User{}).Create(user).Error; err != nil {
				return nil, fmt.Errorf("failed to create oauth user: %w", err)
			}
			return user, nil
		}
		return nil, fmt.Errorf("failed to get oauth user: %w", err)
	}

	updates := map[string]any{
		"display_name": existing.DisplayName,
		"handle":       existing.Handle,
		"avatar_url":   user.AvatarURL,
		"profile_url":  user.ProfileURL,
		"email":        user.Email,
		"update_time":  user.UpdateTime,
	}
	if user.DisplayName != "" {
		updates["display_name"] = user.DisplayName
	}
	if existing.Handle == "" && user.Handle != "" {
		updates["handle"] = user.Handle
	}
	if err = m.mysql.WithContext(ctx).Model(&User{}).
		Where("id = ?", existing.ID).
		Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("failed to update oauth user: %w", err)
	}

	existing.DisplayName = updates["display_name"].(string)
	existing.Handle = updates["handle"].(string)
	existing.AvatarURL = user.AvatarURL
	existing.ProfileURL = user.ProfileURL
	existing.Email = user.Email
	existing.UpdateTime = user.UpdateTime
	return existing, nil
}

func (m *userModel) GetUserByID(ctx context.Context, id uint64) (*User, error) {
	user := &User{}
	if err := m.mysql.WithContext(ctx).Model(&User{}).Where("id = ?", id).First(user).Error; err != nil {
		return nil, err
	}
	return user, nil
}

func (m *userModel) GetMaxNumericHandle(ctx context.Context) (uint64, error) {
	var maxHandle uint64
	if err := m.mysql.WithContext(ctx).Model(&User{}).
		Select("COALESCE(MAX(CAST(handle AS UNSIGNED)), 0)").
		Where("handle REGEXP ? AND CAST(handle AS UNSIGNED) > 0", "^[0-9]+$").
		Scan(&maxHandle).Error; err != nil {
		return 0, fmt.Errorf("failed to get max numeric user handle: %w", err)
	}
	return maxHandle, nil
}

func (m *userModel) ListUsers(ctx context.Context, filter AdminListFilter) ([]AdminListItem, int64, error) {
	applyFilter := func(db *gorm.DB) *gorm.DB {
		if filter.Handle != "" {
			db = db.Where("u.handle = ?", filter.Handle)
		}
		if filter.DisplayName != "" {
			db = db.Where("u.display_name LIKE ?", "%"+filter.DisplayName+"%")
		}
		if filter.Provider != "" {
			db = db.Where("u.provider = ?", filter.Provider)
		}
		switch filter.CommentPermission {
		case "disabled":
			db = db.Where("u.comment_disabled = ? AND (u.comment_disabled_until IS NULL OR u.comment_disabled_until > ?)", true, filter.Now)
		case "normal":
			db = db.Where("u.comment_disabled = ? OR u.comment_disabled_until <= ?", false, filter.Now)
		}
		return db
	}

	countQuery := applyFilter(m.mysql.WithContext(ctx).Table("user AS u"))
	var total int64
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count users: %w", err)
	}

	commentStatsQuery := m.mysql.WithContext(ctx).Table("comment").
		Select("user_id, COUNT(*) AS comment_count, MAX(create_time) AS last_comment_time").
		Group("user_id")

	items := make([]AdminListItem, 0, filter.Limit)
	listQuery := applyFilter(m.mysql.WithContext(ctx).Table("user AS u")).
		Select(`u.id, u.provider, u.provider_user_id, u.display_name, u.handle, u.avatar_url, u.profile_url, u.email,
			u.comment_disabled, u.comment_disabled_reason, u.comment_disabled_until, u.session_version,
			u.create_time, u.update_time, COALESCE(cs.comment_count, 0) AS comment_count, cs.last_comment_time`).
		Joins("LEFT JOIN (?) cs ON cs.user_id = u.id", commentStatsQuery).
		Order("u.create_time DESC, u.id DESC").
		Offset(filter.Offset).
		Limit(filter.Limit)
	if err := listQuery.Scan(&items).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list users: %w", err)
	}
	return items, total, nil
}

func (m *userModel) UpdateCommentPermission(ctx context.Context, id uint64, disabled bool, reason string,
	disabledUntil *time.Time, updateTime time.Time) error {

	updates := map[string]any{
		"comment_disabled":        disabled,
		"comment_disabled_reason": reason,
		"comment_disabled_until":  disabledUntil,
		"update_time":             updateTime,
	}
	result := m.mysql.WithContext(ctx).Model(&User{}).Where("id = ?", id).Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("failed to update user comment permission: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (m *userModel) IncrementSessionVersion(ctx context.Context, id uint64, updateTime time.Time) error {
	result := m.mysql.WithContext(ctx).Model(&User{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"session_version": gorm.Expr("session_version + ?", 1),
			"update_time":     updateTime,
		})
	if result.Error != nil {
		return fmt.Errorf("failed to increment user session version: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

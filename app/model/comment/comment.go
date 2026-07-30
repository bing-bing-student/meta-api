package comment

import (
	"context"
	"fmt"
	"time"

	"meta-api/app/model/article"
	"meta-api/common/utils"
)

const (
	StatusPending  = "pending"
	StatusApproved = "approved"
	StatusRejected = "rejected"
)

func IsValidStatus(status string) bool {
	switch status {
	case StatusPending, StatusApproved, StatusRejected:
		return true
	default:
		return false
	}
}

type Comment struct {
	ID                uint64          `gorm:"primary_key;NOT NULL"`
	ArticleID         uint64          `gorm:"column:article_id;NOT NULL;index:idx_comment_article_status_time,priority:1"`
	ParentID          uint64          `gorm:"column:parent_id;NOT NULL;default:0;index"`
	UserID            uint64          `gorm:"column:user_id;NOT NULL;default:0;index"`
	ReplyToUserID     uint64          `gorm:"column:reply_to_user_id;NOT NULL;default:0;index"`
	ReplyToCommentID  uint64          `gorm:"column:reply_to_comment_id;NOT NULL;default:0;index"`
	AuthorName        string          `gorm:"type:varchar(80);NOT NULL"`
	Content           string          `gorm:"type:varchar(1000);NOT NULL"`
	Status            string          `gorm:"type:varchar(20);NOT NULL;default:pending;index:idx_comment_article_status_time,priority:2"`
	ModerationReasons string          `gorm:"column:moderation_reasons;type:text"`
	IP                string          `gorm:"type:varchar(64)"`
	CreateTime        time.Time       `gorm:"column:create_time;NOT NULL;index:idx_comment_article_status_time,priority:3"`
	UpdateTime        time.Time       `gorm:"column:update_time;NOT NULL"`
	Article           article.Article `gorm:"foreignKey:ArticleID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
}

type AdminListFilter struct {
	ArticleID       uint64
	ArticleTitle    string
	ContentKeyword  string
	AuthorHandle    string
	CreateStartTime *time.Time
	CreateEndTime   *time.Time
	Status          string
	Offset          int
	Limit           int
}

type AdminListItem struct {
	ID                  uint64    `gorm:"column:id"`
	ArticleTitle        string    `gorm:"column:article_title"`
	ParentID            uint64    `gorm:"column:parent_id"`
	ReplyToAuthorName   string    `gorm:"column:reply_to_author_name"`
	ReplyToAuthorHandle string    `gorm:"column:reply_to_author_handle"`
	AuthorHandle        string    `gorm:"column:author_handle"`
	Content             string    `gorm:"column:content"`
	Status              string    `gorm:"column:status"`
	ModerationReasons   string    `gorm:"column:moderation_reasons"`
	IP                  string    `gorm:"column:ip"`
	CreateTime          time.Time `gorm:"column:create_time"`
	UpdateTime          time.Time `gorm:"column:update_time"`
}

type ListItem struct {
	ID                  uint64    `gorm:"column:id"`
	ArticleID           uint64    `gorm:"column:article_id"`
	ParentID            uint64    `gorm:"column:parent_id"`
	UserID              uint64    `gorm:"column:user_id"`
	ReplyToUserID       uint64    `gorm:"column:reply_to_user_id"`
	ReplyToCommentID    uint64    `gorm:"column:reply_to_comment_id"`
	ReplyToAuthorName   string    `gorm:"column:reply_to_author_name"`
	ReplyToAuthorHandle string    `gorm:"column:reply_to_author_handle"`
	ReplyToContent      string    `gorm:"column:reply_to_content"`
	AuthorName          string    `gorm:"column:author_name"`
	AuthorHandle        string    `gorm:"column:author_handle"`
	AvatarURL           string    `gorm:"column:avatar_url"`
	Content             string    `gorm:"column:content"`
	CreateTime          time.Time `gorm:"column:create_time"`
}

func (m *commentModel) CreateComment(ctx context.Context, newComment *Comment) error {
	if err := m.mysql.WithContext(ctx).Model(&Comment{}).Create(newComment).Error; err != nil {
		return fmt.Errorf("failed to create comment: %w", err)
	}
	return nil
}

func (m *commentModel) GetCommentByID(ctx context.Context, id uint64) (*Comment, error) {
	item := &Comment{}
	if err := m.mysql.WithContext(ctx).Model(&Comment{}).Where("id = ?", id).First(item).Error; err != nil {
		return nil, err
	}
	return item, nil
}

func (m *commentModel) GetCommentsByIDs(ctx context.Context, ids []uint64) ([]*Comment, error) {
	items := make([]*Comment, 0, len(ids))
	if len(ids) == 0 {
		return items, nil
	}
	if err := m.mysql.WithContext(ctx).Model(&Comment{}).Where("id IN ?", ids).Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (m *commentModel) ListApprovedByArticleID(ctx context.Context, articleID uint64) ([]ListItem, error) {
	rows := make([]ListItem, 0)
	if err := m.mysql.WithContext(ctx).Model(&Comment{}).Table("comment as c").
		Joins("LEFT JOIN `user` as u ON u.id = c.user_id").
		Joins("LEFT JOIN `user` as ru ON ru.id = c.reply_to_user_id").
		Joins("LEFT JOIN `comment` as rc ON rc.id = c.reply_to_comment_id").
		Where("c.article_id = ? AND c.status = ?", articleID, StatusApproved).
		Select("c.id, c.article_id, c.parent_id, c.user_id, c.reply_to_user_id, c.reply_to_comment_id, ru.display_name as reply_to_author_name, ru.handle as reply_to_author_handle, rc.content as reply_to_content, c.author_name, u.handle as author_handle, u.avatar_url, c.content, c.create_time").
		Order("c.create_time ASC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to list approved comments: %w", err)
	}
	return rows, nil
}

func (m *commentModel) ListApprovedParentsByArticleID(ctx context.Context, articleID uint64, offset int, limit int) ([]ListItem, int64, error) {
	query := m.mysql.WithContext(ctx).Model(&Comment{}).Table("comment as c").
		Where("c.article_id = ? AND c.status = ? AND c.parent_id = 0", articleID, StatusApproved)

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count approved parent comments: %w", err)
	}

	rows := make([]ListItem, 0)
	if total == 0 {
		return rows, 0, nil
	}

	if err := query.
		Joins("LEFT JOIN `user` as u ON u.id = c.user_id").
		Select("c.id, c.article_id, c.parent_id, c.user_id, c.reply_to_user_id, c.reply_to_comment_id, '' as reply_to_author_name, '' as reply_to_author_handle, '' as reply_to_content, c.author_name, u.handle as author_handle, u.avatar_url, c.content, c.create_time").
		Order("c.create_time ASC").
		Offset(offset).
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list approved parent comments: %w", err)
	}
	return rows, total, nil
}

func (m *commentModel) ListApprovedRepliesByParentID(ctx context.Context, parentID uint64, offset int, limit int) ([]ListItem, int64, error) {
	query := m.mysql.WithContext(ctx).Model(&Comment{}).Table("comment as c").
		Where("c.parent_id = ? AND c.status = ?", parentID, StatusApproved)

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count approved comment replies: %w", err)
	}

	rows := make([]ListItem, 0)
	if total == 0 {
		return rows, 0, nil
	}

	if err := query.
		Joins("LEFT JOIN `user` as u ON u.id = c.user_id").
		Joins("LEFT JOIN `user` as ru ON ru.id = c.reply_to_user_id").
		Joins("LEFT JOIN `comment` as rc ON rc.id = c.reply_to_comment_id").
		Select("c.id, c.article_id, c.parent_id, c.user_id, c.reply_to_user_id, c.reply_to_comment_id, ru.display_name as reply_to_author_name, ru.handle as reply_to_author_handle, rc.content as reply_to_content, c.author_name, u.handle as author_handle, u.avatar_url, c.content, c.create_time").
		Order("c.create_time ASC").
		Offset(offset).
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list approved comment replies: %w", err)
	}
	return rows, total, nil
}

func (m *commentModel) ListComments(ctx context.Context, filter AdminListFilter) ([]AdminListItem, int64, error) {
	query := m.mysql.WithContext(ctx).Model(&Comment{}).Table("comment as c").
		Joins("LEFT JOIN article as a ON a.id = c.article_id").
		Joins("LEFT JOIN `user` as u ON u.id = c.user_id").
		Joins("LEFT JOIN `user` as ru ON ru.id = c.reply_to_user_id")

	if filter.ArticleID != 0 {
		query = query.Where("c.article_id = ?", filter.ArticleID)
	}
	if filter.ArticleTitle != "" {
		like := "%" + utils.EscapeLike(filter.ArticleTitle) + "%"
		query = query.Where("a.title LIKE ? COLLATE utf8mb4_general_ci", like)
	}
	if filter.ContentKeyword != "" {
		like := "%" + utils.EscapeLike(filter.ContentKeyword) + "%"
		query = query.Where("c.content LIKE ? COLLATE utf8mb4_general_ci", like)
	}
	if filter.Status != "" {
		query = query.Where("c.status = ?", filter.Status)
	}
	if filter.AuthorHandle != "" {
		query = query.Where("u.handle = ?", filter.AuthorHandle)
	}
	if filter.CreateStartTime != nil {
		query = query.Where("c.create_time >= ?", *filter.CreateStartTime)
	}
	if filter.CreateEndTime != nil {
		query = query.Where("c.create_time <= ?", *filter.CreateEndTime)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count comments: %w", err)
	}

	rows := make([]AdminListItem, 0)
	if total == 0 {
		return rows, 0, nil
	}

	if err := query.
		Select("c.id, a.title as article_title, c.parent_id, ru.display_name as reply_to_author_name, ru.handle as reply_to_author_handle, u.handle as author_handle, c.content, c.status, c.moderation_reasons, c.ip, c.create_time, c.update_time").
		Order("c.create_time DESC").
		Offset(filter.Offset).
		Limit(filter.Limit).
		Find(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list comments: %w", err)
	}
	return rows, total, nil
}

func (m *commentModel) UpdateCommentStatus(ctx context.Context, id uint64, status string, updateTime time.Time) error {
	if err := m.mysql.WithContext(ctx).Model(&Comment{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":      status,
			"update_time": updateTime,
		}).Error; err != nil {
		return fmt.Errorf("failed to update comment status: %w", err)
	}
	return nil
}

func (m *commentModel) DeleteComment(ctx context.Context, id uint64) error {
	if err := m.mysql.WithContext(ctx).Model(&Comment{}).Where("id = ?", id).Delete(&Comment{}).Error; err != nil {
		return fmt.Errorf("failed to delete comment: %w", err)
	}
	return nil
}

func (m *commentModel) DeleteComments(ctx context.Context, ids []uint64) error {
	if len(ids) == 0 {
		return nil
	}
	if err := m.mysql.WithContext(ctx).Model(&Comment{}).Where("id IN ?", ids).Delete(&Comment{}).Error; err != nil {
		return fmt.Errorf("failed to delete comments: %w", err)
	}
	return nil
}

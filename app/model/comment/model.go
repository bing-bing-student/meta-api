package comment

import (
	"context"
	"time"

	"gorm.io/gorm"
)

type Model interface {
	CreateComment(ctx context.Context, newComment *Comment) error
	GetCommentByID(ctx context.Context, id uint64) (*Comment, error)
	GetCommentsByIDs(ctx context.Context, ids []uint64) ([]*Comment, error)
	ListApprovedByArticleID(ctx context.Context, articleID uint64) ([]ListItem, error)
	ListApprovedParentsByArticleID(ctx context.Context, articleID uint64, offset int, limit int) ([]ListItem, int64, error)
	ListApprovedRepliesByParentID(ctx context.Context, parentID uint64, offset int, limit int) ([]ListItem, int64, error)
	ListComments(ctx context.Context, filter AdminListFilter) ([]AdminListItem, int64, error)
	UpdateCommentStatus(ctx context.Context, id uint64, status string, updateTime time.Time) error
	DeleteComment(ctx context.Context, id uint64) error
	DeleteComments(ctx context.Context, ids []uint64) error
}

type commentModel struct {
	mysql *gorm.DB
}

func NewModel(mysql *gorm.DB) Model {
	return &commentModel{mysql: mysql}
}

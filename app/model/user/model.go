package user

import (
	"context"
	"time"

	"gorm.io/gorm"
)

type Model interface {
	UpsertOAuthUser(ctx context.Context, user *User) (*User, error)
	GetUserByID(ctx context.Context, id uint64) (*User, error)
	GetMaxNumericHandle(ctx context.Context) (uint64, error)
	ListUsers(ctx context.Context, filter AdminListFilter) ([]AdminListItem, int64, error)
	UpdateCommentPermission(ctx context.Context, id uint64, disabled bool, reason string, disabledUntil *time.Time, updateTime time.Time) error
	IncrementSessionVersion(ctx context.Context, id uint64, updateTime time.Time) error
}

type userModel struct {
	mysql *gorm.DB
}

func NewModel(mysql *gorm.DB) Model {
	return &userModel{mysql: mysql}
}

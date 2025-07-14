package tag

import (
	"context"

	"gorm.io/gorm"
)

type Model interface {
	CreateTag(ctx context.Context, newTag *Tag) error
	FindTagByName(ctx context.Context, tagName string) (*Tag, error)
}

type tagModel struct {
	mysql *gorm.DB
}

func NewModel(mysql *gorm.DB) Model {
	return &tagModel{mysql: mysql}
}

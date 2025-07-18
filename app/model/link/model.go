package link

import (
	"context"

	"gorm.io/gorm"
)

type Model interface {
	GetLinkList(ctx context.Context) ([]Link, error)
	CreateLink(ctx context.Context, newLink *Link) error
	UpdateLink(ctx context.Context, linkInfo *Link) error
	DeleteLink(ctx context.Context, id uint64) error
}

type linkModel struct {
	mysql *gorm.DB
}

func NewModel(mysql *gorm.DB) Model {
	return &linkModel{mysql: mysql}
}

package article

import (
	"context"

	"gorm.io/gorm"
)

type Model interface {
	CreateArticle(ctx context.Context, newArticle *Article) error
	UpdateArticle(ctx context.Context, articleInfo *Article) error
	GetArticleDetailByID(ctx context.Context, id uint64) (*Detail, error)
	GetArticleListByTagName(ctx context.Context, tagName string) ([]ListByTagName, error)
	DelArticleAndReturnTagName(ctx context.Context, id uint64) (string, error)
}

type articleModel struct {
	mysql *gorm.DB
}

func NewModel(mysql *gorm.DB) Model {
	return &articleModel{mysql: mysql}
}

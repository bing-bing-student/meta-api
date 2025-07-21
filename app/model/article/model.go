package article

import (
	"context"

	"gorm.io/gorm"
)

type Model interface {
	CreateArticle(ctx context.Context, newArticle *Article) error
	UpdateArticle(ctx context.Context, articleInfo *Article) error
	UpdateArticleTagID(ctx context.Context, articleIDList []string, tagID uint64) error
	UpdateArticleViewNum(ctx context.Context, id string, viewNum float64) error
	GetArticleDetailByID(ctx context.Context, id uint64) (*Detail, error)
	GetArticleListByTagName(ctx context.Context, tagName string) ([]ListByTagName, error)
	DelArticleAndReturnTagName(ctx context.Context, id uint64) (string, error)
	SearchArticle(ctx context.Context, word string, limit, offset int) ([]SearchArticle, int64, error)
	GetArticleListByIDList(ctx context.Context, idList []uint64) ([]*Article, error)
}

type articleModel struct {
	mysql *gorm.DB
}

func NewModel(mysql *gorm.DB) Model {
	return &articleModel{mysql: mysql}
}

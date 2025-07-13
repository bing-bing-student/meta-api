package article

import (
	"gorm.io/gorm"
)

type Model interface {
	GetArticleDetailByID(id uint64) (*Detail, error)
	GetTagNameArticleZSetByTagName(tagName string) ([]TagNameArticleZSet, error)
}

type articleModel struct {
	mysql *gorm.DB
}

func NewModel(mysql *gorm.DB) Model {
	return &articleModel{mysql: mysql}
}

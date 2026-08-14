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
	GetArticleDeleteInfo(ctx context.Context, id uint64) (string, error)
	DeleteArticleByID(ctx context.Context, id uint64) error
	SearchArticle(ctx context.Context, word string, limit, offset int) ([]SearchArticle, int64, error)
	GetArticleListByIDList(ctx context.Context, idList []uint64) ([]*Article, error)

	GetArticleList(ctx context.Context, offset, limit int) ([]*Article, error)
	GetArticleCount(ctx context.Context) (int, error)

	CreateArticleDraft(ctx context.Context, draft *Article) error
	UpdateArticleDraft(ctx context.Context, draft *Article) error
	GetArticleDraftByID(ctx context.Context, id uint64) (*Article, error)
	GetArticleDraftDetailByID(ctx context.Context, id uint64) (*Detail, error)
	FindArticleDraftByPublishedID(ctx context.Context, publishedID uint64) (*Article, error)
	CountArticleDrafts(ctx context.Context) (int64, error)
	ListArticleDrafts(ctx context.Context, offset int, limit int) ([]DraftListRecord, int64, error)
	PublishNewArticleDraft(ctx context.Context, draft *Article) error
	PublishArticleDraftToPublished(ctx context.Context, draftID uint64, published *Article) error
	DeleteArticleDraftByID(ctx context.Context, id uint64) error

	ListArticleImageSources(ctx context.Context) ([]ArticleImageSource, error)
	FindArticleImagesByObjectKeys(ctx context.Context, objectKeys []string) (map[string]ArticleImage, error)
	SyncArticleImages(ctx context.Context, images []ArticleImage, references []ArticleImageReference) error
	CreateArticleImage(ctx context.Context, image *ArticleImage) error
	ListArticleImages(ctx context.Context, query ArticleImageQuery) ([]ArticleImageListRecord, int64, error)
	GetArticleImageByID(ctx context.Context, id uint64) (*ArticleImage, error)
	ListArticleImageReferences(ctx context.Context, imageID uint64) ([]ArticleImageReferenceRecord, error)
	CountArticleImageReferences(ctx context.Context, imageID uint64) (int64, error)
	DeleteArticleImage(ctx context.Context, id uint64) error

	ListTimeAndView(ctx context.Context) ([]TimeAndViewZSet, error)
	BatchUpdateViewNum(ctx context.Context, items []ViewNumUpdate) error
}

type articleModel struct {
	mysql *gorm.DB
}

func NewModel(mysql *gorm.DB) Model {
	return &articleModel{mysql: mysql}
}

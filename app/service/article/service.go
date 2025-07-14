package article

import (
	"context"

	"github.com/redis/go-redis/v9"
	"github.com/sony/sonyflake"
	"go.uber.org/zap"

	"meta-api/app/model/article"
	"meta-api/app/model/tag"
	"meta-api/common/types"
	"meta-api/config"
)

// Service 文章服务接口
type Service interface {
	AdminGetArticleList(ctx context.Context, request *types.AdminGetArticleListRequest) (*types.AdminGetArticleListResponse, error)
	AdminGetArticleDetail(ctx context.Context, request *types.AdminGetArticleDetailRequest) (*types.AdminGetArticleDetailResponse, error)
	AdminAddArticle(ctx context.Context, request *types.AdminAddArticleRequest) error
	AdminUpdateArticle(ctx context.Context, request *types.AdminUpdateArticleRequest) error
	AdminDeleteArticle(ctx context.Context, request *types.AdminDeleteArticleRequest) error

	UserGetArticleList(ctx context.Context, request *types.UserGetArticleListRequest) (*types.UserGetArticleListResponse, error)
	UserGetArticleDetail(ctx context.Context, request *types.UserGetArticleDetailRequest) (*types.UserGetArticleDetailResponse, error)
	UserSearchArticle(ctx context.Context, request *types.UserSearchArticleRequest) (*types.UserSearchArticleResponse, error)
	UserGetHotArticle(ctx context.Context) (*types.UserGetHotArticleResponse, error)
	UserGetTimeline(ctx context.Context) (*types.GetTimelineResponse, error)
}

// articleService 文章服务
type articleService struct {
	config       *config.Config
	logger       *zap.Logger
	idGenerator  *sonyflake.Sonyflake
	redis        *redis.Client
	articleModel article.Model
	tagModel     tag.Model
}

// NewService 创建服务实例
func NewService(config *config.Config, logger *zap.Logger,
	idGenerator *sonyflake.Sonyflake, redis *redis.Client,
	articleModel article.Model, tagModel tag.Model) Service {

	return &articleService{
		config:       config,
		logger:       logger,
		idGenerator:  idGenerator,
		redis:        redis,
		articleModel: articleModel,
		tagModel:     tagModel,
	}
}

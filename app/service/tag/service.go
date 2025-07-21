package tag

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

// Service 标签服务接口
type Service interface {
	AdminGetTagList(ctx context.Context) (*types.AdminGetTagListResponse, error)
	AdminGetArticleListByTag(ctx context.Context, request *types.AdminGetArticleListByTagRequest) (*types.AdminGetArticleListByTagResponse, error)
	AdminUpdateTag(ctx context.Context, request *types.AdminUpdateTagRequest) error

	UserGetTagList(ctx context.Context) (*types.UserGetTagListResponse, error)
	UserGetArticleListByTag(ctx context.Context, request *types.UserGetArticleListByTagRequest) (*types.UserGetArticleListByTagResponse, error)
}

// tagService 标签服务
type tagService struct {
	config       *config.Config
	logger       *zap.Logger
	idGenerator  *sonyflake.Sonyflake
	redis        *redis.Client
	tagModel     tag.Model
	articleModel article.Model
}

// NewService 创建服务实例
func NewService(config *config.Config, logger *zap.Logger, idGenerator *sonyflake.Sonyflake, redis *redis.Client, tagModel tag.Model, articleModel article.Model) Service {
	return &tagService{
		config:       config,
		logger:       logger,
		idGenerator:  idGenerator,
		redis:        redis,
		tagModel:     tagModel,
		articleModel: articleModel,
	}
}

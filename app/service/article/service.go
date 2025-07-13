package article

import (
	"context"

	"github.com/redis/go-redis/v9"
	"github.com/sony/sonyflake"
	"go.uber.org/zap"

	"meta-api/app/model/article"
	"meta-api/common/types"
	"meta-api/config"
)

// Service 文章服务接口
type Service interface {
	AdminGetArticleList(ctx context.Context, req *types.AdminGetArticleListRequest, resp *types.AdminGetArticleListResponse) (err error)
}

// articleService 文章服务
type articleService struct {
	config      *config.Config
	logger      *zap.Logger
	idGenerator *sonyflake.Sonyflake
	redis       *redis.Client
	model       article.Model
}

// NewService 创建服务实例
func NewService(config *config.Config, logger *zap.Logger, idGenerator *sonyflake.Sonyflake, redis *redis.Client, model article.Model) Service {
	return &articleService{
		config:      config,
		logger:      logger,
		idGenerator: idGenerator,
		redis:       redis,
		model:       model,
	}
}

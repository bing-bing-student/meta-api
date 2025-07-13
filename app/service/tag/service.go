package tag

import (
	"github.com/redis/go-redis/v9"
	"github.com/sony/sonyflake"
	"go.uber.org/zap"

	"meta-api/app/model/tag"
	"meta-api/config"
)

// Service 标签服务接口
type Service interface {
}

// tagService 标签服务
type tagService struct {
	config      *config.Config
	logger      *zap.Logger
	idGenerator *sonyflake.Sonyflake
	redis       *redis.Client
	model       tag.Model
}

// NewService 创建服务实例
func NewService(config *config.Config, logger *zap.Logger, idGenerator *sonyflake.Sonyflake, redis *redis.Client, model tag.Model) Service {
	return &tagService{
		config:      config,
		logger:      logger,
		idGenerator: idGenerator,
		redis:       redis,
		model:       model,
	}
}

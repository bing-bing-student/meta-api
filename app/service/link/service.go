package link

import (
	"github.com/redis/go-redis/v9"
	"github.com/sony/sonyflake"
	"go.uber.org/zap"

	"meta-api/app/model/link"
	"meta-api/config"
)

// Service 友链服务接口
type Service interface {
}

// linkService 友链服务
type linkService struct {
	config      *config.Config
	logger      *zap.Logger
	idGenerator *sonyflake.Sonyflake
	redis       *redis.Client
	model       link.Model
}

// NewService 创建服务实例
func NewService(config *config.Config, logger *zap.Logger, idGenerator *sonyflake.Sonyflake, redis *redis.Client, model link.Model) Service {
	return &linkService{
		config:      config,
		logger:      logger,
		idGenerator: idGenerator,
		redis:       redis,
		model:       model,
	}
}

package service

import (
	"github.com/redis/go-redis/v9"
	"github.com/sony/sonyflake"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"meta-api/config"
	"meta-api/internal/app/model"
)

type Service struct {
	config      *config.Config       // 配置
	logger      *zap.Logger          // 日志
	idGenerator *sonyflake.Sonyflake // ID 生成器
	model       *model.Model         // 数据模型
}

func NewService(config *config.Config, logger *zap.Logger, idGenerator *sonyflake.Sonyflake, db *gorm.DB, redis *redis.Client) *Service {
	return &Service{
		config:      config,
		logger:      logger,
		idGenerator: idGenerator,
		model:       model.NewModel(db, redis),
	}
}

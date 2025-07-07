package service

import (
	"github.com/sony/sonyflake"
	"go.uber.org/zap"

	"meta-api/config"
	"meta-api/internal/app/model"
)

type Service struct {
	config      *config.Config       // 配置
	logger      *zap.Logger          // 日志
	idGenerator *sonyflake.Sonyflake // ID 生成器
	model       *model.Model         // 数据模型
}

func NewService(config *config.Config, logger *zap.Logger, idGenerator *sonyflake.Sonyflake, model *model.Model) *Service {
	return &Service{
		config:      config,
		logger:      logger,
		idGenerator: idGenerator,
		model:       model,
	}
}

package model

import (
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type Model struct {
	mysql *gorm.DB      // MySQL 客户端
	redis *redis.Client // Redis 客户端
}

// NewModel 创建模型实例
func NewModel(mysql *gorm.DB, redis *redis.Client) *Model {
	return &Model{mysql: mysql, redis: redis}
}

package di

import (
	"fmt"

	"github.com/redis/go-redis/v9"
	"github.com/sony/sonyflake"
	"go.uber.org/dig"
	"go.uber.org/zap"
	"gorm.io/gorm"

	adminHandler "meta-api/app/handler/admin"
	articleHandler "meta-api/app/handler/article"
	linkHandler "meta-api/app/handler/link"
	tagHandler "meta-api/app/handler/tag"

	adminModel "meta-api/app/model/admin"
	articleModel "meta-api/app/model/article"
	linkModel "meta-api/app/model/link"
	tagModel "meta-api/app/model/tag"

	adminService "meta-api/app/service/admin"
	articleService "meta-api/app/service/article"
	linkService "meta-api/app/service/link"
	tagService "meta-api/app/service/tag"

	"meta-api/bootstrap"
	"meta-api/config"
)

func BuildContainer(bs *bootstrap.Bootstrap) (*dig.Container, error) {
	container := dig.New()

	// 提供基础依赖
	baseProviders := []interface{}{
		func() *config.Config { return bs.Config },
		func() *zap.Logger { return bs.Logger },
		func() *sonyflake.Sonyflake { return bs.IDGenerator },
		func() *gorm.DB { return bs.MySQL },
		func() *redis.Client { return bs.Redis },
	}
	for _, provider := range baseProviders {
		if err := container.Provide(provider); err != nil {
			return nil, fmt.Errorf("failed to provide base: %w", err)
		}
	}

	// 注册Handler层依赖
	handlerProviders := []interface{}{
		adminHandler.NewHandler,
		articleHandler.NewHandler,
		linkHandler.NewHandler,
		tagHandler.NewHandler,
	}
	for _, provider := range handlerProviders {
		if err := container.Provide(provider); err != nil {
			return nil, fmt.Errorf("failed to provide handler: %w", err)
		}
	}

	// 注册Model层依赖
	modelProviders := []interface{}{
		adminModel.NewModel,
		articleModel.NewModel,
		linkModel.NewModel,
		tagModel.NewModel,
	}
	for _, provider := range modelProviders {
		if err := container.Provide(provider); err != nil {
			return nil, fmt.Errorf("failed to provide model: %w", err)
		}
	}

	// 注册Service层依赖
	serviceProviders := []interface{}{
		adminService.NewService,
		articleService.NewService,
		linkService.NewService,
		tagService.NewService,
	}
	for _, provider := range serviceProviders {
		if err := container.Provide(provider); err != nil {
			return nil, fmt.Errorf("failed to provide service: %w", err)
		}
	}

	return container, nil
}

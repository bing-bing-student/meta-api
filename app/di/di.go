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
	viewLogHandler "meta-api/app/handler/viewlog"

	adminModel "meta-api/app/model/admin"
	articleModel "meta-api/app/model/article"
	linkModel "meta-api/app/model/link"
	tagModel "meta-api/app/model/tag"

	adminService "meta-api/app/service/admin"
	articleService "meta-api/app/service/article"
	linkService "meta-api/app/service/link"
	tagService "meta-api/app/service/tag"
	viewLogService "meta-api/app/service/viewlog"

	"meta-api/bootstrap"
	"meta-api/config"
	"meta-api/pkg/edgeone"
	"meta-api/pkg/keymanager"
	"meta-api/pkg/revalidator"
)

// BuildContainer 依赖注入容器
func BuildContainer(bs *bootstrap.Bootstrap) (*dig.Container, error) {
	container := dig.New()

	// 注册基础依赖
	baseProviders := []interface{}{
		func() *config.Config { return bs.Config },
		func() *zap.Logger { return bs.Logger },
		func() *sonyflake.Sonyflake { return bs.IDGenerator },
		func() *gorm.DB { return bs.MySQL },
		func() *redis.Client { return bs.Redis },
		func(logger *zap.Logger) *revalidator.Client { return revalidator.New(logger) },
		func(logger *zap.Logger) *edgeone.Client { return edgeone.New(logger) },
		func(logger *zap.Logger) *keymanager.Manager { return keymanager.New(logger) },
	}
	for _, provider := range baseProviders {
		if err := container.Provide(provider); err != nil {
			return nil, fmt.Errorf("failed to provide base: %w", err)
		}
	}

	// 注册 Handler 层依赖
	handlerProviders := []interface{}{
		adminHandler.NewHandler,
		articleHandler.NewHandler,
		linkHandler.NewHandler,
		tagHandler.NewHandler,
		viewLogHandler.NewHandler,
	}
	for _, provider := range handlerProviders {
		if err := container.Provide(provider); err != nil {
			return nil, fmt.Errorf("failed to provide handler: %w", err)
		}
	}

	// 注册 Model 层依赖
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

	// 注册 Service 层依赖
	serviceProviders := []interface{}{
		adminService.NewService,
		articleService.NewService,
		linkService.NewService,
		tagService.NewService,
		viewLogService.NewService,
	}
	for _, provider := range serviceProviders {
		if err := container.Provide(provider); err != nil {
			return nil, fmt.Errorf("failed to provide service: %w", err)
		}
	}

	return container, nil
}

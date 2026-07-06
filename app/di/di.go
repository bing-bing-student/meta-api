package di

import (
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sony/sonyflake"
	"go.uber.org/dig"
	"go.uber.org/zap"
	"gorm.io/gorm"

	adminHandler "meta-api/app/handler/admin"
	articleHandler "meta-api/app/handler/article"
	linkHandler "meta-api/app/handler/link"
	shareHandler "meta-api/app/handler/share"
	tagHandler "meta-api/app/handler/tag"
	viewLogHandler "meta-api/app/handler/viewlog"

	adminModel "meta-api/app/model/admin"
	articleModel "meta-api/app/model/article"
	linkModel "meta-api/app/model/link"
	tagModel "meta-api/app/model/tag"

	adminService "meta-api/app/service/admin"
	articleService "meta-api/app/service/article"
	linkService "meta-api/app/service/link"
	shareService "meta-api/app/service/share"
	tagService "meta-api/app/service/tag"
	viewLogService "meta-api/app/service/viewlog"

	"meta-api/bootstrap"
	"meta-api/common/guard"
	"meta-api/config"
	"meta-api/pkg/edgeone"
	"meta-api/pkg/keymanager"
	"meta-api/pkg/sitemap"
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
		func(logger *zap.Logger) *edgeone.Client { return edgeone.New(logger) },
		func(logger *zap.Logger) *keymanager.Manager { return keymanager.New(logger) },
		func(logger *zap.Logger) *sitemap.Client { return sitemap.New(logger) },
		// guard.Store：底层 Redis 抽象。同时被 guard.Engine 与 share.Service 复用，
		// 单独 provide 是为了让 share.Service 也能拿到（共用同一份 store 实例）。
		func(rdb *redis.Client, logger *zap.Logger) guard.Store {
			return guard.NewRedisStore(rdb, logger)
		},
		// guard.Engine：风控守卫新引擎。
		// 缺省 BuildHashes 为空 + SkipHMACWhenEmpty=true 即可平滑灰度（仍校验 RSA/AES/TLV）；
		// 上线全量后通过 config.guard.build_hashes 注入白名单并把 skip_hmac_when_empty 切回 false。
		func(cfg *config.Config, logger *zap.Logger, store guard.Store,
			km *keymanager.Manager) (guard.Engine, error) {
			gc := cfg.GuardConfig
			registry := guard.NewBuildHashRegistry()
			skipHMAC := true
			if gc != nil {
				if err := registerBuildHashes(registry, gc.BuildHashes); err != nil {
					return nil, fmt.Errorf("guard build_hashes invalid: %w", err)
				}
				skipHMAC = gc.SkipHMACWhenEmpty
			}
			return guard.NewEngine(guard.EngineConfig{
				KeyManager:        km,
				Store:             store,
				Logger:            logger,
				BuildHashes:       registry,
				SkipHMACWhenEmpty: skipHMAC,
			})
		},
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
		shareHandler.NewHandler,
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
		shareService.NewService,
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

// registerBuildHashes 把配置中的 hex 字符串数组依次注册到 BuildHashRegistry。
//
// 任一条目格式不合法（非 16 字符 hex）即返回错误，让进程启动失败而不是默默放过。
// expireAt 留空表示永不过期；上线后可考虑配合配置中心动态下发 + 过期时间做老版本自动下线。
func registerBuildHashes(registry *guard.BuildHashRegistry, hashes []string) error {
	for _, h := range hashes {
		if h == "" {
			continue
		}
		if err := registry.RegisterFromHex(h, time.Time{}); err != nil {
			return fmt.Errorf("register build hash %q: %w", h, err)
		}
	}
	return nil
}

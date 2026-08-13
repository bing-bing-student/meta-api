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
	commentHandler "meta-api/app/handler/comment"
	jsonshareHandler "meta-api/app/handler/jsonshare"
	linkHandler "meta-api/app/handler/link"
	tagHandler "meta-api/app/handler/tag"
	userAuthHandler "meta-api/app/handler/userauth"
	viewLogHandler "meta-api/app/handler/viewlog"

	adminModel "meta-api/app/model/admin"
	articleModel "meta-api/app/model/article"
	commentModel "meta-api/app/model/comment"
	linkModel "meta-api/app/model/link"
	tagModel "meta-api/app/model/tag"
	userModel "meta-api/app/model/user"

	adminService "meta-api/app/service/admin"
	articleService "meta-api/app/service/article"
	commentService "meta-api/app/service/comment"
	jsonshareService "meta-api/app/service/jsonshare"
	linkService "meta-api/app/service/link"
	tagService "meta-api/app/service/tag"
	userAuthService "meta-api/app/service/userauth"
	viewLogService "meta-api/app/service/viewlog"

	"meta-api/bootstrap"
	"meta-api/common/guard"
	"meta-api/common/guard/keymanager"
	"meta-api/config"
	"meta-api/pkg/cdn"
	"meta-api/pkg/cos"
	"meta-api/pkg/sitemap"
)

// BuildContainer 依赖注入容器
func BuildContainer(bs *bootstrap.Bootstrap) (*dig.Container, error) {
	container := dig.New()

	// 注册基础依赖
	baseProviders := []any{
		func() *config.Config { return bs.Config },
		func() *zap.Logger { return bs.Logger },
		func() *sonyflake.Sonyflake { return bs.IDGenerator },
		func() *gorm.DB { return bs.MySQL },
		func() *redis.Client { return bs.Redis },
		func() *keymanager.Manager { return bs.KeyManager },
		func(logger *zap.Logger) *cdn.Client { return cdn.New(logger, bs.Context()) },
		func(cfg *config.Config, logger *zap.Logger) *cos.Client {
			return cos.New(cfg.ArticleImageCOSSnapshot(), logger)
		},
		func(logger *zap.Logger) *sitemap.Client { return sitemap.New(logger, bs.Context()) },
		func(rdb *redis.Client, logger *zap.Logger) guard.Store { return guard.NewRedisStore(rdb, logger) },
		newGuardEngine,
	}
	for _, provider := range baseProviders {
		if err := container.Provide(provider); err != nil {
			return nil, fmt.Errorf("failed to provide base: %w", err)
		}
	}

	// 注册 Handler 层依赖
	handlerProviders := []any{
		adminHandler.NewHandler,
		articleHandler.NewHandler,
		commentHandler.NewHandler,
		jsonshareHandler.NewHandler,
		linkHandler.NewHandler,
		tagHandler.NewHandler,
		userAuthHandler.NewHandler,
		viewLogHandler.NewHandler,
	}
	for _, provider := range handlerProviders {
		if err := container.Provide(provider); err != nil {
			return nil, fmt.Errorf("failed to provide handler: %w", err)
		}
	}

	// 注册 Model 层依赖
	modelProviders := []any{
		adminModel.NewModel,
		articleModel.NewModel,
		commentModel.NewModel,
		linkModel.NewModel,
		tagModel.NewModel,
		userModel.NewModel,
	}
	for _, provider := range modelProviders {
		if err := container.Provide(provider); err != nil {
			return nil, fmt.Errorf("failed to provide model: %w", err)
		}
	}

	// 注册 Service 层依赖
	serviceProviders := []any{
		adminService.NewService,
		articleService.NewService,
		commentService.NewService,
		jsonshareService.NewService,
		linkService.NewService,
		tagService.NewService,
		userAuthService.NewService,
		viewLogService.NewService,
	}
	for _, provider := range serviceProviders {
		if err := container.Provide(provider); err != nil {
			return nil, fmt.Errorf("failed to provide service: %w", err)
		}
	}

	return container, nil
}

// newGuardEngine 构造风控守卫引擎。
//
// 缺省 BuildHashes 为空 + SkipHMACWhenEmpty=true 即可平滑灰度（仍校验 RSA/AES/TLV）；
// 上线全量后通过 config.guard.build_hashes 注入白名单并把 skip_hmac_when_empty 切回 false。
func newGuardEngine(cfg *config.Config, logger *zap.Logger, store guard.Store,
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

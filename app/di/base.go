package di

import (
	"fmt"

	"github.com/redis/go-redis/v9"
	"github.com/sony/sonyflake"
	"go.uber.org/dig"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"meta-api/bootstrap"
	"meta-api/common/guard"
	"meta-api/common/guard/keymanager"
	"meta-api/config"
	"meta-api/pkg/cdn"
	"meta-api/pkg/cos"
	"meta-api/pkg/sitemap"
)

func validateBootstrap(bs *bootstrap.Bootstrap) error {
	if bs == nil {
		return fmt.Errorf("bootstrap is nil")
	}
	if bs.Config == nil {
		return fmt.Errorf("bootstrap config is nil")
	}
	if bs.Logger == nil {
		return fmt.Errorf("bootstrap logger is nil")
	}
	if bs.IDGenerator == nil {
		return fmt.Errorf("bootstrap id generator is nil")
	}
	if bs.MySQL == nil {
		return fmt.Errorf("bootstrap mysql is nil")
	}
	if bs.Redis == nil {
		return fmt.Errorf("bootstrap redis is nil")
	}
	if bs.KeyManager == nil {
		return fmt.Errorf("bootstrap key manager is nil")
	}
	return nil
}

func registerBaseProviders(container *dig.Container, bs *bootstrap.Bootstrap) error {
	providers := []provider{
		{name: "config", constructor: func() *config.Config { return bs.Config }},
		{name: "logger", constructor: func() *zap.Logger { return bs.Logger }},
		{name: "id generator", constructor: func() *sonyflake.Sonyflake { return bs.IDGenerator }},
		{name: "mysql", constructor: func() *gorm.DB { return bs.MySQL }},
		{name: "redis", constructor: func() *redis.Client { return bs.Redis }},
		{name: "key manager", constructor: func() *keymanager.Manager { return bs.KeyManager }},
		{name: "cdn client", constructor: func(logger *zap.Logger) *cdn.Client { return cdn.New(logger, bs.Context()) }},
		{name: "cos client", constructor: func(cfg *config.Config, logger *zap.Logger) *cos.Client {
			return cos.New(cfg.ArticleImageCOSSnapshot(), logger)
		}},
		{name: "sitemap client", constructor: func(logger *zap.Logger) *sitemap.Client {
			return sitemap.New(logger, bs.Context())
		}},
		{name: "guard store", constructor: func(rdb *redis.Client, logger *zap.Logger) guard.Store {
			return guard.NewRedisStore(rdb, logger)
		}},
		{name: "guard engine", constructor: newGuardEngine},
	}

	for _, provider := range providers {
		if err := provide(container, provider.name, provider.constructor); err != nil {
			return err
		}
	}
	return nil
}

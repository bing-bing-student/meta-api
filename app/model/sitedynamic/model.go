package sitedynamic

import (
	"context"
	"time"

	"gorm.io/gorm"
)

type Model interface {
	ListSiteDynamics(ctx context.Context) ([]SiteDynamic, error)
	ListPublishedSiteDynamics(ctx context.Context) ([]SiteDynamic, error)
	CreateSiteDynamic(ctx context.Context, item *SiteDynamic) error
	UpdateSiteDynamic(ctx context.Context, item *SiteDynamic) error
	DeleteSiteDynamic(ctx context.Context, id uint64) error
	ReorderSiteDynamics(ctx context.Context, ids []uint64, updateTime time.Time) error
}

type siteDynamicModel struct {
	mysql *gorm.DB
}

func NewModel(mysql *gorm.DB) Model {
	return &siteDynamicModel{mysql: mysql}
}

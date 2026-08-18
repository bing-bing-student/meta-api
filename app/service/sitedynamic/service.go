package sitedynamic

import (
	"context"

	"github.com/redis/go-redis/v9"
	"github.com/sony/sonyflake"
	"go.uber.org/zap"

	siteDynamicModel "meta-api/app/model/sitedynamic"
	"meta-api/common/types"
)

type Service interface {
	AdminGetSiteDynamicList(ctx context.Context) (*types.AdminGetSiteDynamicListResponse, error)
	AdminAddSiteDynamic(ctx context.Context, request *types.AdminAddSiteDynamicRequest) error
	AdminUpdateSiteDynamic(ctx context.Context, request *types.AdminUpdateSiteDynamicRequest) error
	AdminReorderSiteDynamics(ctx context.Context, request *types.AdminReorderSiteDynamicRequest) error
	AdminDeleteSiteDynamic(ctx context.Context, request *types.AdminDeleteSiteDynamicRequest) error

	UserGetSiteDynamicList(ctx context.Context) (*types.UserGetSiteDynamicListResponse, error)
}

type siteDynamicService struct {
	logger      *zap.Logger
	idGenerator *sonyflake.Sonyflake
	redis       *redis.Client
	model       siteDynamicModel.Model
}

func NewService(logger *zap.Logger, idGenerator *sonyflake.Sonyflake, redis *redis.Client,
	model siteDynamicModel.Model) Service {
	return &siteDynamicService{
		logger:      logger,
		idGenerator: idGenerator,
		redis:       redis,
		model:       model,
	}
}

package admin

import (
	"context"

	"github.com/redis/go-redis/v9"
	"github.com/sony/sonyflake"
	"go.uber.org/zap"

	"meta-api/app/model/admin"
	"meta-api/common/types"
	"meta-api/config"
)

// Service 管理员服务接口
type Service interface {
	GenerateToken(userClaims *types.UserClaims) (*types.TokenDetails, error)
	SendSMSCode(ctx context.Context, request *types.SendSMSCodeRequest) error
	SMSCodeLogin(ctx context.Context, request *types.SMSCodeLoginRequest) (*types.SMSCodeLoginResponse, error)
	AccountLogin(ctx context.Context, request *types.AccountLoginRequest) (*types.AccountLoginResponse, error)
	BindDynamicCode(ctx context.Context, request *types.BindDynamicCodeRequest) (*types.BindDynamicCodeResponse, error)
	VerifyDynamicCode(ctx context.Context, request *types.VerifyDynamicCodeRequest) (*types.VerifyDynamicCodeResponse, error)
}

// adminService 管理员服务实现
type adminService struct {
	config      *config.Config
	logger      *zap.Logger
	idGenerator *sonyflake.Sonyflake
	redis       *redis.Client
	model       admin.Model
}

// NewService 创建服务实例
func NewService(config *config.Config, logger *zap.Logger, idGenerator *sonyflake.Sonyflake, redis *redis.Client, model admin.Model) Service {
	return &adminService{
		config:      config,
		logger:      logger,
		idGenerator: idGenerator,
		redis:       redis,
		model:       model,
	}
}

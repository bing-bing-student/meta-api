package admin

import (
	"context"

	"github.com/redis/go-redis/v9"
	"github.com/sony/sonyflake"
	"go.uber.org/zap"

	"meta-api/app/model/admin"
	userModel "meta-api/app/model/user"
	"meta-api/common/ratelimit"
	"meta-api/common/types"
	"meta-api/config"
)

// Service 管理员服务接口
type Service interface {
	GenerateToken(ctx context.Context, userClaims *types.UserClaims) (*types.TokenDetails, error)
	RefreshToken(ctx context.Context, refreshToken string) (*types.TokenDetails, error)
	RevokeRefreshToken(ctx context.Context, refreshToken string) error
	SendSMSCode(ctx context.Context, request *types.SendSMSCodeRequest) error
	SMSCodeLogin(ctx context.Context, request *types.SMSCodeLoginRequest) (*types.SMSCodeLoginResponse, error)
	AccountLogin(ctx context.Context, request *types.AccountLoginRequest) (*types.AccountLoginResponse, error)
	BindDynamicCode(ctx context.Context, request *types.BindDynamicCodeRequest) (*types.BindDynamicCodeResponse, error)
	VerifyDynamicCode(ctx context.Context, request *types.VerifyDynamicCodeRequest) (*types.VerifyDynamicCodeResponse, error)
	AdminUpdateAboutMe(ctx context.Context, request *types.UpdateAboutMeRequest) error
	AdminGetUserList(ctx context.Context, request *types.AdminGetUserListRequest) (*types.AdminGetUserListResponse, error)
	AdminUpdateUserCommentPermission(ctx context.Context, request *types.AdminUpdateUserCommentPermissionRequest) error
	AdminForceUserLogout(ctx context.Context, request *types.AdminForceUserLogoutRequest) error

	UserGetAboutMe(ctx context.Context) (*types.GetAboutMeResponse, error)
	UserSubmitBugFeedback(ctx context.Context, request *types.SubmitBugFeedbackRequest) error
}

// adminService 管理员服务实现
type adminService struct {
	config      *config.Config
	logger      *zap.Logger
	idGenerator *sonyflake.Sonyflake
	redis       *redis.Client
	limiter     *ratelimit.Limiter
	model       admin.Model
	userModel   userModel.Model
}

// NewService 创建服务实例
func NewService(config *config.Config, logger *zap.Logger, idGenerator *sonyflake.Sonyflake, redis *redis.Client,
	model admin.Model, userModel userModel.Model) Service {
	return &adminService{
		config:      config,
		logger:      logger,
		idGenerator: idGenerator,
		redis:       redis,
		limiter:     ratelimit.NewRedisLimiter(redis),
		model:       model,
		userModel:   userModel,
	}
}

package userauth

import (
	"context"
	"errors"

	"github.com/redis/go-redis/v9"
	"github.com/sony/sonyflake"
	"go.uber.org/zap"

	commentModel "meta-api/app/model/comment"
	userModel "meta-api/app/model/user"
	"meta-api/common/types"
	"meta-api/config"
)

var (
	ErrInvalidOAuthState    = errors.New("invalid oauth state")
	ErrOAuthProviderMissing = errors.New("oauth provider is not configured")
	ErrInvalidOAuthRedirect = errors.New("invalid oauth redirect")
	ErrUserAuthFailed       = errors.New("user auth failed")
	ErrUserHandleTaken      = errors.New("user handle is taken")
	ErrUserSessionInvalid   = errors.New("user session invalid")
)

type Service interface {
	BuildOAuthLoginURL(ctx context.Context, request *types.OAuthLoginRequest) (*OAuthLoginResult, error)
	HandleOAuthCallback(ctx context.Context, request *types.OAuthCallbackRequest) (*types.OAuthCallbackResponse, string, error)
	GetCurrentUser(ctx context.Context, userID string, sessionVersion int64) (*types.PublicUserInfo, error)
}

type OAuthLoginResult struct {
	AuthorizationURL string
	FlowBinding      string
}

type userAuthService struct {
	logger       *zap.Logger
	idGenerator  *sonyflake.Sonyflake
	redis        *redis.Client
	config       *config.Config
	commentModel commentModel.Model
	userModel    userModel.Model
}

func NewService(logger *zap.Logger, idGenerator *sonyflake.Sonyflake, redis *redis.Client,
	cfg *config.Config, commentModel commentModel.Model, userModel userModel.Model) Service {
	return &userAuthService{
		logger:       logger,
		idGenerator:  idGenerator,
		redis:        redis,
		config:       cfg,
		commentModel: commentModel,
		userModel:    userModel,
	}
}

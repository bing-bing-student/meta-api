package comment

import (
	"context"
	"errors"

	"github.com/redis/go-redis/v9"
	"github.com/sony/sonyflake"
	"go.uber.org/zap"

	articleModel "meta-api/app/model/article"
	commentModel "meta-api/app/model/comment"
	userModel "meta-api/app/model/user"
	"meta-api/common/ratelimit"
	"meta-api/common/types"
	"meta-api/config"
)

var (
	ErrInvalidComment        = errors.New("invalid comment")
	ErrCommentNotFound       = errors.New("comment not found")
	ErrCommentUnauthorized   = errors.New("comment unauthorized")
	ErrCommentForbidden      = errors.New("comment forbidden")
	ErrCommentSessionInvalid = errors.New("comment session invalid")
)

type Service interface {
	UserGetCommentList(ctx context.Context, request *types.UserGetCommentListRequest) (*types.UserGetCommentListResponse, error)
	UserGetCommentReplyList(ctx context.Context, request *types.UserGetCommentReplyListRequest) (*types.UserGetCommentReplyListResponse, error)
	UserAddComment(ctx context.Context, request *types.UserAddCommentRequest) (*types.UserAddCommentResponse, error)

	AdminGetCommentList(ctx context.Context, request *types.AdminGetCommentListRequest) (*types.AdminGetCommentListResponse, error)
	AdminUpdateCommentStatus(ctx context.Context, request *types.AdminUpdateCommentStatusRequest) error
	AdminDeleteComment(ctx context.Context, request *types.AdminDeleteCommentRequest) error
}

type commentService struct {
	config       *config.Config
	logger       *zap.Logger
	idGenerator  *sonyflake.Sonyflake
	redis        *redis.Client
	limiter      *ratelimit.Limiter
	commentModel commentModel.Model
	articleModel articleModel.Model
	userModel    userModel.Model
}

func NewService(config *config.Config, logger *zap.Logger, idGenerator *sonyflake.Sonyflake, redis *redis.Client,
	commentModel commentModel.Model, articleModel articleModel.Model, userModel userModel.Model) Service {
	return &commentService{
		config:       config,
		logger:       logger,
		idGenerator:  idGenerator,
		redis:        redis,
		limiter:      ratelimit.NewRedisLimiter(redis),
		commentModel: commentModel,
		articleModel: articleModel,
		userModel:    userModel,
	}
}

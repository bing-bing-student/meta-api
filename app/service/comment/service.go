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
	commentModeration "meta-api/app/service/comment/moderation"
	"meta-api/common/ratelimit"
	"meta-api/common/types"
	"meta-api/config"
)

var (
	ErrInvalidComment         = errors.New("invalid comment")
	ErrCommentNotFound        = errors.New("comment not found")
	ErrCommentUnauthorized    = errors.New("comment unauthorized")
	ErrCommentForbidden       = errors.New("comment forbidden")
	ErrCommentSessionInvalid  = errors.New("comment session invalid")
	ErrCommentAlreadyReported = errors.New("comment already reported")
)

// Service 定义评论模块面向用户端和管理端的业务能力。
// 各方法的 ctx 用于取消数据库或缓存操作，request 承载已完成基础绑定的请求；返回响应数据或可由处理层映射的业务错误。
type Service interface {
	UserGetCommentList(ctx context.Context, request *types.UserGetCommentListRequest) (*types.UserGetCommentListResponse, error)
	UserGetCommentReplyList(ctx context.Context, request *types.UserGetCommentReplyListRequest) (*types.UserGetCommentReplyListResponse, error)
	UserAddComment(ctx context.Context, request *types.UserAddCommentRequest) (*types.UserAddCommentResponse, error)
	UserReportComment(ctx context.Context, request *types.UserReportCommentRequest) (*types.UserReportCommentResponse, error)
	UserGetCommentReportStatus(ctx context.Context, request *types.UserGetCommentReportStatusRequest) (*types.UserGetCommentReportStatusResponse, error)

	AdminGetCommentList(ctx context.Context, request *types.AdminGetCommentListRequest) (*types.AdminGetCommentListResponse, error)
	AdminGetCommentDetail(ctx context.Context, request *types.AdminGetCommentDetailRequest) (*types.AdminGetCommentDetailResponse, error)
	AdminUpdateCommentStatus(ctx context.Context, request *types.AdminUpdateCommentStatusRequest) error
	AdminReviewComment(ctx context.Context, request *types.AdminReviewCommentRequest) error
	AdminDeleteComment(ctx context.Context, request *types.AdminDeleteCommentRequest) error
	AdminPreviewCommentModeration(ctx context.Context, request *types.AdminPreviewCommentModerationRequest) (*types.AdminPreviewCommentModerationResponse, error)
	AdminSubmitCommentModerationFeedback(ctx context.Context, request *types.AdminSubmitCommentModerationFeedbackRequest) error
	AdminGetCommentReportList(ctx context.Context, request *types.AdminGetCommentReportListRequest) (*types.AdminGetCommentReportListResponse, error)
	AdminHandleCommentReport(ctx context.Context, request *types.AdminHandleCommentReportRequest) error
}

// commentService 聚合评论业务所需的配置、日志、ID 生成器、缓存、限流器、审核器和数据模型。
// 实例由 NewService 创建，并作为 Service 接口的唯一实现。
type commentService struct {
	config       *config.Config
	logger       *zap.Logger
	idGenerator  *sonyflake.Sonyflake
	redis        *redis.Client
	limiter      *ratelimit.Limiter
	moderator    *commentModeration.Moderator
	commentModel commentModel.Model
	articleModel articleModel.Model
	userModel    userModel.Model
}

// NewService 使用配置、日志器、ID 生成器、Redis 及评论、文章和用户模型创建评论服务。
// 输入依赖由应用启动阶段注入；返回实现用户评论、后台审核和举报处理能力的 Service。
func NewService(config *config.Config, logger *zap.Logger, idGenerator *sonyflake.Sonyflake, redis *redis.Client,
	commentModel commentModel.Model, articleModel articleModel.Model, userModel userModel.Model) Service {
	return &commentService{
		config:       config,
		logger:       logger,
		idGenerator:  idGenerator,
		redis:        redis,
		limiter:      ratelimit.NewRedisLimiter(redis),
		moderator:    commentModeration.NewModerator(config, logger, redis),
		commentModel: commentModel,
		articleModel: articleModel,
		userModel:    userModel,
	}
}

package article

import (
	"context"
	"sync"

	"github.com/redis/go-redis/v9"
	"github.com/robfig/cron/v3"
	"github.com/sony/sonyflake"
	"go.uber.org/zap"

	"meta-api/app/model/article"
	"meta-api/app/model/tag"
	"meta-api/common/types"
	"meta-api/config"
	"meta-api/pkg/cdn"
	"meta-api/pkg/cos"
	"meta-api/pkg/sitemap"
)

// Service 文章服务接口
type Service interface {
	AdminGetArticleList(ctx context.Context, request *types.AdminGetArticleListRequest) (*types.AdminGetArticleListResponse, error)
	AdminGetArticleDetail(ctx context.Context, request *types.AdminGetArticleDetailRequest) (*types.AdminGetArticleDetailResponse, error)
	AdminAddArticle(ctx context.Context, request *types.AdminAddArticleRequest) (*types.AdminSaveArticleResponse, error)
	AdminUpdateArticle(ctx context.Context, request *types.AdminUpdateArticleRequest) (*types.AdminSaveArticleResponse, error)
	AdminDeleteArticle(ctx context.Context, request *types.AdminDeleteArticleRequest) error
	AdminGetArticleDraftList(ctx context.Context, request *types.AdminGetArticleDraftListRequest) (*types.AdminGetArticleDraftListResponse, error)
	AdminGetArticleDraftDetail(ctx context.Context, request *types.AdminGetArticleDraftDetailRequest) (*types.AdminGetArticleDraftDetailResponse, error)
	AdminSaveArticleDraft(ctx context.Context, request *types.AdminSaveArticleDraftRequest) (*types.AdminSaveArticleResponse, error)
	AdminPublishArticleDraft(ctx context.Context, request *types.AdminPublishArticleDraftRequest) (*types.AdminSaveArticleResponse, error)
	AdminDeleteArticleDraft(ctx context.Context, request *types.AdminDeleteArticleDraftRequest) error
	AdminUploadArticleImage(ctx context.Context, fileName string, contentType string, content []byte) (*types.AdminUploadArticleImageResponse, error)
	AdminGetArticleImageList(ctx context.Context, request *types.AdminGetArticleImageListRequest) (*types.AdminGetArticleImageListResponse, error)
	AdminGetArticleImageDetail(ctx context.Context, request *types.AdminGetArticleImageDetailRequest) (*types.AdminGetArticleImageDetailResponse, error)
	AdminDeleteArticleImage(ctx context.Context, request *types.AdminDeleteArticleImageRequest) error

	UserGetArticleList(ctx context.Context, request *types.UserGetArticleListRequest) (*types.UserGetArticleListResponse, error)
	UserGetArticleDetail(ctx context.Context, request *types.UserGetArticleDetailRequest) (*types.UserGetArticleDetailResponse, error)
	UserSearchArticle(ctx context.Context, request *types.UserSearchArticleRequest) (*types.UserSearchArticleResponse, error)
	UserGetHotArticle(ctx context.Context) (*types.UserGetHotArticleResponse, error)
	UserGetTimeline(ctx context.Context) (*types.GetTimelineResponse, error)

	WarmUpCache(ctx context.Context) error
	PersistViewCount(ctx context.Context) error
	RegisterCronJobs(c *cron.Cron) ([]cron.EntryID, error)
}

// articleService 文章服务
type articleService struct {
	config       *config.Config
	logger       *zap.Logger
	idGenerator  *sonyflake.Sonyflake
	redis        *redis.Client
	articleModel article.Model
	tagModel     tag.Model
	cdn          *cdn.Client
	imageStore   *cos.Client
	sitemap      *sitemap.Client
	// articleCacheMu 串行化进程内的文章排序缓存重建，避免缓存失效时并发回源 MySQL。
	articleCacheMu sync.Mutex
}

// NewService 创建服务实例
func NewService(config *config.Config, logger *zap.Logger,
	idGenerator *sonyflake.Sonyflake, redis *redis.Client,
	articleModel article.Model, tagModel tag.Model,
	cdnClient *cdn.Client, imageStore *cos.Client, sm *sitemap.Client) Service {

	return &articleService{
		config:       config,
		logger:       logger,
		idGenerator:  idGenerator,
		redis:        redis,
		articleModel: articleModel,
		tagModel:     tagModel,
		cdn:          cdnClient,
		imageStore:   imageStore,
		sitemap:      sm,
	}
}

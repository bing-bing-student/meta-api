package article

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"meta-api/app/service/article"
)

type Handler interface {
	AdminGetArticleList(c *gin.Context)
	AdminGetArticleDetail(c *gin.Context)
	AdminAddArticle(c *gin.Context)
	AdminUpdateArticle(c *gin.Context)
	AdminDeleteArticle(c *gin.Context)

	UserGetArticleList(c *gin.Context)
	UserSearchArticle(c *gin.Context)
	UserGetHotArticle(c *gin.Context)
	UserGetArticleDetail(c *gin.Context)
	UserGetTimeline(c *gin.Context)
}

type articleHandler struct {
	logger  *zap.Logger
	service article.Service
}

func NewHandler(logger *zap.Logger, service article.Service) Handler {
	return &articleHandler{
		logger:  logger,
		service: service,
	}
}

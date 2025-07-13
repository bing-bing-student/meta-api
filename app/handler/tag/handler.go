package tag

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"meta-api/app/service/tag"
)

type Handler interface {
	AdminGetTagList(c *gin.Context)
	AdminGetArticleListByTag(c *gin.Context)
	AdminUpdateTag(c *gin.Context)

	UserGetTagList(c *gin.Context)
	UserGetArticleListByTag(c *gin.Context)
}

type tagHandler struct {
	logger  *zap.Logger
	service tag.Service
}

func NewHandler(logger *zap.Logger, service tag.Service) Handler {
	return &tagHandler{
		logger:  logger,
		service: service,
	}
}

package sitedynamic

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"meta-api/app/service/sitedynamic"
)

type Handler interface {
	AdminGetSiteDynamicList(c *gin.Context)
	AdminAddSiteDynamic(c *gin.Context)
	AdminUpdateSiteDynamic(c *gin.Context)
	AdminReorderSiteDynamics(c *gin.Context)
	AdminDeleteSiteDynamic(c *gin.Context)

	UserGetSiteDynamicList(c *gin.Context)
}

type siteDynamicHandler struct {
	logger  *zap.Logger
	service sitedynamic.Service
}

func NewHandler(logger *zap.Logger, service sitedynamic.Service) Handler {
	return &siteDynamicHandler{
		logger:  logger,
		service: service,
	}
}

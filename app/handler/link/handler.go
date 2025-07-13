package link

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"meta-api/app/service/link"
)

type Handler interface {
	AdminGetLinkList(c *gin.Context)
	AdminAddLink(c *gin.Context)
	AdminUpdateLink(c *gin.Context)
	AdminDeleteLink(c *gin.Context)

	UserGetLinkList(c *gin.Context)
}

type linkHandler struct {
	logger  *zap.Logger
	service link.Service
}

func NewHandler(logger *zap.Logger, service link.Service) Handler {
	return &linkHandler{
		logger:  logger,
		service: service,
	}
}

package admin

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"meta-api/app/service/admin"
)

type Handler interface {
	RefreshTokenToLogin(c *gin.Context)
}

type adminHandler struct {
	logger  *zap.Logger
	service admin.Service
}

func NewHandler(logger *zap.Logger, service admin.Service) Handler {
	return &adminHandler{
		logger:  logger,
		service: service,
	}
}

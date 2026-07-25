package userauth

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"meta-api/app/service/userauth"
)

type Handler interface {
	OAuthLogin(c *gin.Context)
	OAuthCallback(c *gin.Context)
	Me(c *gin.Context)
	Logout(c *gin.Context)
}

type userAuthHandler struct {
	logger  *zap.Logger
	service userauth.Service
}

func NewHandler(logger *zap.Logger, service userauth.Service) Handler {
	return &userAuthHandler{
		logger:  logger,
		service: service,
	}
}

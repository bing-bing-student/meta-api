package admin

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"meta-api/app/service/admin"
)

type Handler interface {
	RefreshToken(c *gin.Context)
	Logout(c *gin.Context)
	SendSMSCode(c *gin.Context)
	SMSCodeLogin(c *gin.Context)
	AccountLogin(c *gin.Context)
	BindDynamicCode(c *gin.Context)
	VerifyDynamicCode(c *gin.Context)

	AdminUpdateAboutMe(c *gin.Context)
	AdminGetUserList(c *gin.Context)
	AdminUpdateUserCommentPermission(c *gin.Context)
	AdminForceUserLogout(c *gin.Context)
	UserGetAboutMe(c *gin.Context)
	UserSubmitBugFeedback(c *gin.Context)
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

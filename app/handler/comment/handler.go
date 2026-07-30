package comment

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"meta-api/app/service/comment"
)

type Handler interface {
	UserGetCommentList(c *gin.Context)
	UserGetCommentReplyList(c *gin.Context)
	UserAddComment(c *gin.Context)
	UserReportComment(c *gin.Context)
	UserGetCommentReportStatus(c *gin.Context)

	AdminGetCommentList(c *gin.Context)
	AdminUpdateCommentStatus(c *gin.Context)
	AdminDeleteComment(c *gin.Context)
	AdminGetCommentReportList(c *gin.Context)
	AdminHandleCommentReport(c *gin.Context)
}

type commentHandler struct {
	logger  *zap.Logger
	service comment.Service
}

func NewHandler(logger *zap.Logger, service comment.Service) Handler {
	return &commentHandler{
		logger:  logger,
		service: service,
	}
}

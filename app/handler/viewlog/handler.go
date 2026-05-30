package viewlog

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"meta-api/app/service/viewlog"
)

// Handler 浏览量打点 HTTP 入口。
type Handler interface {
	PostViewLog(c *gin.Context)
}

type viewLogHandler struct {
	logger  *zap.Logger
	service viewlog.Service
}

// NewHandler 构造打点 handler 实例。
func NewHandler(logger *zap.Logger, service viewlog.Service) Handler {
	return &viewLogHandler{
		logger:  logger,
		service: service,
	}
}

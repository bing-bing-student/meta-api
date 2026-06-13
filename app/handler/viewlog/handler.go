package viewlog

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"meta-api/app/service/viewlog"
	"meta-api/common/guard"
)

// Handler 浏览量打点 HTTP 入口。
type Handler interface {
	PostViewLog(c *gin.Context)
}

type viewLogHandler struct {
	logger  *zap.Logger
	service viewlog.Service
	// engine 风控守卫引擎；构造期保证非 nil。
	engine guard.Engine
}

// NewHandler 构造打点 handler 实例。
//
// engine 必填，为 nil 时直接 panic（构造期 fail-fast）。
func NewHandler(logger *zap.Logger, service viewlog.Service, engine guard.Engine) Handler {
	if engine == nil {
		panic("viewlog.NewHandler: guard.Engine is required")
	}
	return &viewLogHandler{
		logger:  logger,
		service: service,
		engine:  engine,
	}
}

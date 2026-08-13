// Package jsonshare HTTP 入口：JSON 分享创建场景的风控守卫预检 + 一次性 token 消费。
//
// 路由：
//
//	POST /user/share/precheck     —— 浏览器直接调用，body 为 envelope（octet-stream）
//	POST /user/share/consume      —— 仅 Nuxt SSR 内网调用，header X-Guard-Token
//
// 文件分布：
//
//	handler.go —— Handler interface + 构造
//	jsonshare.go —— Precheck / Consume 两个端点的 HTTP 适配
package jsonshare

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	jsonshareService "meta-api/app/service/jsonshare"
)

// Handler JSON 分享场景风控守卫 HTTP 入口。
type Handler interface {
	// Precheck POST /user/share/precheck
	// body: envelope (octet-stream, 最大 16KB，与 guard.MaxBodyBytes 对齐)
	Precheck(c *gin.Context)

	// Consume POST /user/share/consume
	// header: X-Guard-Token
	// 仅供 Nuxt SSR 内网调用；响应体含明文 fingerprint。
	Consume(c *gin.Context)
}

type jsonShareHandler struct {
	logger  *zap.Logger
	service jsonshareService.Service
}

// NewHandler 构造 JSON 分享场景 handler。service 必填。
func NewHandler(logger *zap.Logger, service jsonshareService.Service) Handler {
	return &jsonShareHandler{
		logger:  logger,
		service: service,
	}
}

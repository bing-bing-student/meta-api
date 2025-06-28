package server

import (
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"meta-api/config"
	"meta-api/internal/pkg/middlewares"
)

// SetupRouter 设置路由
func SetupRouter(logger *zap.Logger, , conf *config.Config) *gin.Engine {
	r := gin.New()

	// 禁用代理
	if err := r.SetTrustedProxies(nil); err != nil {
		logger.Error("error set trusted proxy", zap.Error(err))
	}

	corsConfig := cors.Config{
		AllowOrigins: []string{"http://localhost:3000", "http://localhost:4000"},
		AllowMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders: []string{
			"Content-Type",
			"Content-Length",
			"Accept-Encoding",
			"Authorization",
			"accept",
			"origin",
			"Cache-Control",
			"x-client-id",
		},
		MaxAge:           24 * time.Hour,
		AllowCredentials: true,
	}

	// 添加中间件
	r.Use(
		middlewares.TimeoutMiddleware(2*time.Second),
		cors.New(corsConfig),
		middlewares.GinLogger(logger),
		middlewares.GinRecovery(logger, true),
	)

	// 路由分组配置...
	// (保持您原有的路由分组逻辑)

	return r
}

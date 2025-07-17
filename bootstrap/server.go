package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type HTTPServer struct {
	server *http.Server
	logger *zap.Logger
}

// NewHTTPServer 初始化HTTP服务
func NewHTTPServer(host, port string, handler *gin.Engine, logger *zap.Logger) *HTTPServer {
	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%s", host, port),
		Handler:      handler,
		ReadTimeout:  3 * time.Second, // 读取请求超时时间
		WriteTimeout: 3 * time.Second, // 写响应超时时间
		IdleTimeout:  5 * time.Second, // 空闲连接超时时间
	}

	return &HTTPServer{
		server: server,
		logger: logger,
	}
}

// Start 启动HTTP服务
func (s *HTTPServer) Start() {
	go func() {
		if err := s.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error("HTTP server listen error", zap.Error(err))
		}
	}()
}

// Stop 停止HTTP服务
func (s *HTTPServer) Stop(ctx context.Context) {
	if err := s.server.Shutdown(ctx); err != nil {
		s.logger.Error("HTTP server shutdown error", zap.Error(err))
	}
}

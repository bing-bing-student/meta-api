package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type HTTPServer struct {
	*http.Server
	logger *zap.Logger
}

// NewHTTPServer 初始化HTTP服务
func NewHTTPServer(port int, host string, handler *gin.Engine, logger *zap.Logger) *HTTPServer {
	return &HTTPServer{
		Server: &http.Server{
			Addr:    fmt.Sprintf("%s:%d", host, port),
			Handler: handler,
		},
		logger: logger,
	}
}

// Start 启动HTTP服务
func (s *HTTPServer) Start() {
	go func() {
		if err := s.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error("HTTP server listen error", zap.Error(err))
		}
	}()
}

// Stop 停止HTTP服务
func (s *HTTPServer) Stop(ctx context.Context) error {
	return s.Shutdown(ctx)
}

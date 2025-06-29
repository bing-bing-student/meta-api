package server

import (
	"context"
	"os"
	"time"

	"go.uber.org/zap"

	"meta-api/internal/app/router"
	"meta-api/internal/bootstrap"
)

// Server 服务管理器
type Server struct {
	bootstrap *bootstrap.Application
	http      *bootstrap.HTTPServer
}

// NewServer 创建服务管理器
func NewServer(bootstrapApp *bootstrap.Application) *Server {
	// 初始化路由
	r := router.SetUpRouter(bootstrapApp.Logger)

	return &Server{
		bootstrap: bootstrapApp,
		http:      bootstrap.NewHTTPServer(os.Getenv("HTTP_PORT"), bootstrapApp.Config.Server.Host, r, bootstrapApp.Logger),
	}
}

// Start 启动服务
func (s *Server) Start() {
	s.http.Start()
	s.bootstrap.Logger.Info("HTTP server started", zap.String("address", s.http.Addr))
}

// Stop 停止服务
func (s *Server) Stop(ctx context.Context) {
	// 设置优雅关闭超时时间
	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := s.http.Stop(shutdownCtx); err != nil {
		s.bootstrap.Logger.Error("HTTP server shutdown error", zap.Error(err))
	} else {
		s.bootstrap.Logger.Info("HTTP server stopped gracefully")
	}
}

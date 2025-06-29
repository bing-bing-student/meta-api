package app

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"
)

// LifecycleManager 应用生命周期管理器
type LifecycleManager struct {
	app    *App
	logger *zap.Logger
}

// NewLifecycleManager 创建生命周期管理器
func NewLifecycleManager(app *App, logger *zap.Logger) *LifecycleManager {
	return &LifecycleManager{
		app:    app,
		logger: logger,
	}
}

// RunWithGracefulShutdown 运行应用并处理优雅关闭
func (lm *LifecycleManager) RunWithGracefulShutdown() {
	// 启动应用
	go lm.app.Run()

	// 设置信号监听
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	// 创建优雅关闭上下文
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 执行优雅关闭
	lm.logger.Info("Shutting down application gracefully...")
	lm.app.Stop(ctx)

	// 检查超时
	select {
	case <-ctx.Done():
		lm.logger.Warn("Shutdown timeout exceeded, forcing exit")
	}

	lm.logger.Info("Application stopped")
}

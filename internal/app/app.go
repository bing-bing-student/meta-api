package app

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"meta-api/internal/app/server"
	"meta-api/internal/app/task"
	"meta-api/internal/bootstrap"
)

// App 应用实例
type App struct {
	bootstrap *bootstrap.Application
	server    *server.Server
}

// New 创建应用实例
func New(bootstrapApp *bootstrap.Application) *App {
	return &App{
		bootstrap: bootstrapApp,
		server:    server.NewServer(bootstrapApp),
	}
}

// Run 运行应用核心服务
func (a *App) Run() {
	// 启动基础组件
	a.bootstrap.Start()

	// 执行缓存预热
	task.WarmUp(a.bootstrap)

	// 启动HTTP服务器
	a.server.Start()
}

// Stop 停止应用
func (a *App) Stop(ctx context.Context) {
	// 执行数据持久化
	task.PersistData(a.bootstrap)

	// 停止HTTP服务器
	a.server.Stop(ctx)

	// 停止基础组件
	a.bootstrap.Stop(ctx)
}

// RunWithGracefulShutdown 运行应用并处理优雅关闭
func (a *App) RunWithGracefulShutdown() {
	logger := a.bootstrap.Logger

	// 启动应用核心服务
	go a.Run()

	// 设置信号监听
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// 等待中断信号
	<-quit

	// 创建关闭上下文
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 执行优雅关闭
	a.Stop(ctx)

	// 检查超时
	select {
	case <-ctx.Done():
		logger.Error("Shutdown timeout exceeded, forcing exit")
	}
}

// GetLogger 获取日志记录器
func (a *App) GetLogger() *zap.Logger {
	return a.bootstrap.Logger
}

package app

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"
	"time"

	"meta-api/internal/app/router"
	"meta-api/internal/bootstrap"
)

// App 应用核心管理器
type App struct {
	bootstrap *bootstrap.Application
	http      *bootstrap.HTTPServer // 直接持有HTTP服务器
}

// NewApp 创建应用实例
func NewApp(bootstrapApp *bootstrap.Application) *App {
	// 初始化路由
	r := router.SetUpRouter(bootstrapApp.Logger)

	return &App{
		bootstrap: bootstrapApp,
		http:      bootstrap.NewHTTPServer(os.Getenv("HTTP_HOST"), os.Getenv("HTTP_PORT"), r, bootstrapApp.Logger),
	}
}

// Run 启动应用核心服务
func (a *App) Run() {
	// 启动基础组件
	a.bootstrap.Start()

	// 执行缓存预热
	//cron_task.WarmUp(a.bootstrap)

	// 启动HTTP服务器
	a.http.Start()
}

// Stop 停止应用
func (a *App) Stop(ctx context.Context) {
	// 停止HTTP服务器
	a.http.Stop(ctx)

	// 执行数据持久化
	//cron_task.PersistData(a.bootstrap)

	// 停止基础组件
	a.bootstrap.Stop()
}

// RunWithGracefulShutdown 运行应用并处理优雅关闭
func (a *App) RunWithGracefulShutdown() {
	logger := a.bootstrap.Logger

	// 启动应用核心服务
	go a.Run()

	// 设置信号监听
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 创建通道接收关闭完成信号
	done := make(chan struct{})
	go func() {
		a.Stop(ctx)
		close(done)
	}()

	// 同时监听关闭完成和超时
	select {
	case <-done:
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			logger.Error("Shutdown timeout exceeded, forcing exit")
		}
	}
}

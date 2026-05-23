package app

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"meta-api/app/di"
	"meta-api/app/router"
	articleService "meta-api/app/service/article"
	"meta-api/bootstrap"
)

// Application 应用核心管理器
type Application struct {
	bootstrap      *bootstrap.Bootstrap
	http           *bootstrap.HTTPServer
	articleService articleService.Service
}

// NewApp 创建应用实例
func NewApp(bs *bootstrap.Bootstrap) *Application {
	// 在 app 层统一构建依赖注入容器，
	// 既给 router 使用，也给 app 自己拿 service 用，确保单例一致
	container, err := di.BuildContainer(bs)
	if err != nil {
		bs.Logger.Fatal("failed to build di container", zap.Error(err))
	}

	var artSvc articleService.Service
	if err = container.Invoke(func(s articleService.Service) { artSvc = s }); err != nil {
		bs.Logger.Fatal("failed to resolve article service", zap.Error(err))
	}

	r := router.SetUpRouter(bs, container)
	httpServer := bootstrap.NewHTTPServer(os.Getenv("HTTP_HOST"), os.Getenv("HTTP_PORT"), r, bs.Logger)

	return &Application{
		bootstrap:      bs,
		http:           httpServer,
		articleService: artSvc,
	}
}

// Run 启动应用核心服务
func (a *Application) Run(ctx context.Context) {
	// 启动基础组件（仅启动 cron 调度器，业务任务在下面统一注册）
	a.bootstrap.Start()

	// 缓存预热（业务实现下沉到 service 层，app 层只负责调度与日志）
	if err := a.articleService.WarmUpCache(ctx); err != nil {
		a.bootstrap.Logger.Error("failed to warm up article cache", zap.Error(err))
	}

	// 注册定时任务：把 Redis 中的浏览量周期性回写 MySQL
	if ids, err := a.articleService.RegisterCronJobs(a.bootstrap.Cron); err != nil {
		a.bootstrap.Logger.Error("failed to register article cron jobs", zap.Error(err))
	} else {
		a.bootstrap.CronEntryIDList = &ids
	}

	// 启动HTTP服务器
	a.http.Start()
}

// Stop 停止应用
func (a *Application) Stop(ctx context.Context) {
	// 1. 停止 HTTP，确保不再有新请求产生浏览增量
	a.http.Stop(ctx)

	// 2. 关闭时兜底落盘：把 Redis 中最新浏览量同步至 MySQL
	//    防止两次 cron 之间崩溃 / 关闭导致的增量丢失
	if err := a.articleService.PersistViewCount(ctx); err != nil {
		a.bootstrap.Logger.Error("failed to persist article view count", zap.Error(err))
	}

	// 3. 停止基础组件（关闭 cron / MySQL / Redis 连接）
	a.bootstrap.Stop()
}

// RunWithGracefulShutdown app启停生命周期管理
func (a *Application) RunWithGracefulShutdown() {
	// 创建启动上下文
	runCtx, runCancel := context.WithCancel(context.Background())
	defer runCancel()

	// 启动应用
	go a.Run(runCtx)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	// 创建关闭上下文
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	// 执行关闭
	done := make(chan struct{})
	go func() {
		a.Stop(shutdownCtx)
		close(done)
	}()

	// 等待关闭完成或超时
	select {
	case <-done:
	case <-shutdownCtx.Done():
		if errors.Is(shutdownCtx.Err(), context.DeadlineExceeded) {
			a.bootstrap.Logger.Error("Graceful shutdown timed out")
		}
	}
}

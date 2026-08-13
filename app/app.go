package app

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"

	"meta-api/app/di"
	"meta-api/app/router"
	articleService "meta-api/app/service/article"
	"meta-api/bootstrap"
)

// Application 应用核心管理器
type Application struct {
	bootstrap     *bootstrap.Bootstrap
	http          *bootstrap.HTTPServer
	startupTasks  []startupTask
	cronTasks     []cronTask
	shutdownTasks []shutdownTask
}

// startupTask 应用启动时执行的任务
type startupTask struct {
	name string
	run  func(context.Context) error
}

// cronTask 应用 cron 任务
type cronTask struct {
	name     string
	register func(*cron.Cron) ([]cron.EntryID, error)
}

// shutdownTask 应用关闭时执行的任务
type shutdownTask struct {
	name string
	run  func(context.Context) error
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

	r, err := router.SetUpRouter(bs, container)
	if err != nil {
		bs.Logger.Fatal("failed to setup router", zap.Error(err))
	}
	httpServer := bootstrap.NewHTTPServer(bs.RuntimeEnv.HTTPHost, bs.RuntimeEnv.HTTPPort, r, bs.Logger)

	return &Application{
		bootstrap: bs,
		http:      httpServer,
		startupTasks: []startupTask{
			{name: "warm up article cache", run: artSvc.WarmUpCache},
		},
		cronTasks: []cronTask{
			{name: "register article cron jobs", register: artSvc.RegisterCronJobs},
		},
		shutdownTasks: []shutdownTask{
			{name: "persist article view count", run: artSvc.PersistViewCount},
		},
	}
}

// Run 启动应用核心服务
func (a *Application) Run(ctx context.Context) error {
	for _, task := range a.startupTasks {
		if err := task.run(ctx); err != nil {
			a.bootstrap.Logger.Error("startup task failed",
				zap.String("task", task.name), zap.Error(err))
		}
	}

	var cronEntryIDs []cron.EntryID
	for _, task := range a.cronTasks {
		ids, err := task.register(a.bootstrap.Cron)
		if err != nil {
			a.bootstrap.Logger.Error("cron task registration failed",
				zap.String("task", task.name), zap.Error(err))
			continue
		}
		cronEntryIDs = append(cronEntryIDs, ids...)
	}
	if len(cronEntryIDs) > 0 {
		a.bootstrap.CronEntryIDList = &cronEntryIDs
	}

	// cron 任务注册完成后再启动调度器，避免启动时序和注册时序交错
	a.bootstrap.Start()

	if err := a.http.Start(); err != nil {
		a.bootstrap.StopCron(ctx)
		return err
	}
	return nil
}

// Stop 停止应用
func (a *Application) Stop(ctx context.Context) {
	// 1. 停止 HTTP，确保不再有新请求产生浏览增量
	a.http.Stop(ctx)

	// 2. 停止 cron，避免停机兜底任务和定时任务并发写入
	a.bootstrap.StopCron(ctx)

	// 3. 执行业务停机任务，例如把内存/缓存中的增量兜底落盘
	for _, task := range a.shutdownTasks {
		if err := task.run(ctx); err != nil {
			a.bootstrap.Logger.Error("shutdown task failed", zap.String("task", task.name), zap.Error(err))
		}
	}

	// 4. 关闭基础资源连接
	a.bootstrap.CloseResources()
}

// RunWithGracefulShutdown app启停生命周期管理
func (a *Application) RunWithGracefulShutdown() {
	// 创建启动上下文
	runCtx, runCancel := context.WithCancel(context.Background())
	defer runCancel()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// 启动应用
	if err := a.Run(runCtx); err != nil {
		a.bootstrap.Logger.Fatal("failed to run app", zap.Error(err))
	}
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

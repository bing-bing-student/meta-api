package app

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"meta-api/internal/app/model"
	"meta-api/internal/bootstrap"
)

// Application 应用核心管理器
type Application struct {
	bootstrap *bootstrap.Bootstrap
	http      *bootstrap.HTTPServer
}

// NewApp 创建应用实例
func NewApp(bs *bootstrap.Bootstrap) *Application {
	r := SetUpRouter(bs)
	httpServer := bootstrap.NewHTTPServer(os.Getenv("HTTP_HOST"), os.Getenv("HTTP_PORT"), r, bs.Logger)

	return &Application{
		bootstrap: bs,
		http:      httpServer,
	}
}

// Run 启动应用核心服务
func (a *Application) Run(ctx context.Context) {
	// 启动基础组件
	a.bootstrap.Start(ctx)

	// 缓存预热
	if err := WarmUp(ctx, a.bootstrap); err != nil {
		a.bootstrap.Logger.Error("failed to warm up", zap.Error(err))
	}

	// 启动HTTP服务器
	a.http.Start()
}

// Stop 停止应用
func (a *Application) Stop(ctx context.Context) {
	// 停止HTTP服务器
	a.http.Stop(ctx)

	// 数据持久化
	if err := PersistData(ctx, a.bootstrap); err != nil {
		a.bootstrap.Logger.Error("failed to persist data", zap.Error(err))
	}

	// 停止基础组件
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

// WarmUp 缓存预热
func WarmUp(ctx context.Context, bs *bootstrap.Bootstrap) error {
	redisClient := bs.Redis
	mysqlClient := bs.MySQL
	logger := bs.Logger

	// 删除 Redis 中 article:time:ZSet 有序集合
	if err := redisClient.Del(ctx, "article:time:ZSet").Err(); err != nil {
		logger.Error("failed to delete article:time:ZSet", zap.Error(err))
		return err
	}
	// 删除 Redis 中 article:view:ZSet 有序集合
	if err := redisClient.Del(ctx, "article:view:ZSet").Err(); err != nil {
		logger.Error("failed to delete article:view:ZSet", zap.Error(err))
		return err
	}

	// 获取所有文章数据
	timeAndViewData := make([]model.TimeAndViewZSet, 0)
	if err := mysqlClient.Model(&model.Article{}).Select("id", "view_num", "create_time").Find(&timeAndViewData).Error; err != nil {
		logger.Error("failed to get timeAndViewData", zap.Error(err))
		return err
	}

	// 初始化 article:time:ZSet 和 article:view:ZSet 有序集合
	for _, data := range timeAndViewData {
		if err := redisClient.ZAdd(ctx, "article:time:ZSet", redis.Z{
			Score:  float64(data.CreateTime.UnixNano() / int64(time.Millisecond)),
			Member: data.ID,
		}).Err(); err != nil {
			logger.Error("failed to add article:time:ZSet", zap.Error(err))
			return err
		}
		if err := redisClient.ZAdd(ctx, "article:view:ZSet", redis.Z{
			Score:  float64(data.ViewNum),
			Member: data.ID,
		}).Err(); err != nil {
			logger.Error("failed to add article:view:ZSet", zap.Error(err))
			return err
		}
	}
	return nil
}

// PersistData 数据持久化
func PersistData(ctx context.Context, bs *bootstrap.Bootstrap) error {
	redisClient := bs.Redis
	mysqlClient := bs.MySQL
	logger := bs.Logger

	// 获取缓存数据
	list, err := redisClient.ZRangeWithScores(ctx, "article:view:ZSet", 0, -1).Result()
	if err != nil {
		logger.Error("failed to query article:view:ZSet", zap.Error(err))
		return err
	}

	// 批量更新 - 文章浏览量同步至MySQL
	for _, element := range list {
		articleID := element.Member.(string)
		viewNum := int(element.Score)
		if err = mysqlClient.Model(&model.Article{}).Where("id = ?", articleID).Update("view_num", viewNum).Error; err != nil {
			logger.Error("failed to update article view num", zap.Error(err))
			return err
		}
	}
	return nil
}

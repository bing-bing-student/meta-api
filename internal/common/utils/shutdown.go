package utils

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"
)

// GracefulShutdown 优雅停机
func GracefulShutdown(srv *http.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	if err := srv.Shutdown(ctx); err != nil {
		global.Logger.Fatal("Server forced to shutdown", zap.Error(err))
	} else {
		global.Logger.Info("Server gracefully shut down")
	}
	// 更新MySQL中的浏览量
	//article.UpdateViewNumToMySQLByScheduledTask()
}

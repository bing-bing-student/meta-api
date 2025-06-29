package main

import (
	"meta-api/internal/app"
	"meta-api/internal/bootstrap"
)

func main() {
	// 初始化基础组件
	bootstrapApp := bootstrap.New()

	// 创建应用实例
	application := app.New(bootstrapApp)

	// 创建生命周期管理器并运行
	lifecycle := app.NewLifecycleManager(application, application.GetLogger())
	lifecycle.RunWithGracefulShutdown()
}

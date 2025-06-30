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

	// 运行应用并处理优雅关闭
	application.RunWithGracefulShutdown()
}

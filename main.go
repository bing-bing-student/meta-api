package main

import (
	"meta-api/internal/bootstrap"
)

func main() {
	// 初始化基础组件
	bootstrapApp := bootstrap.New()

	// 创建应用实例
	application := app.New(bootstrapApp)

	// 设置应用（路由、定时任务等）
	application.Setup()

	// 运行应用
	application.Run()
}

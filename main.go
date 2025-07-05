package main

import (
	"meta-api/internal/app"
	"meta-api/internal/bootstrap"
)

func main() {
	// 初始化基础组件
	bootstrapApp := bootstrap.New()
	bootstrapApp.InitConfig()      // 初始化配置
	bootstrapApp.InitLogger()      // 初始化日志
	bootstrapApp.InitIDGenerator() // 初始化ID生成器
	bootstrapApp.InitCron()        // 初始化定时任务
	bootstrapApp.InitMySQL()       // 创建MySQL客户端
	bootstrapApp.InitRedis()       // 创建Redis客户端

	// 创建应用实例
	application := app.NewApp(bootstrapApp)

	// 运行应用并处理优雅关闭
	application.RunWithGracefulShutdown()
}

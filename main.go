package main

import (
	"log"

	"meta-api/app"
	"meta-api/bootstrap"
)

var runtimeEnv *bootstrap.RuntimeEnv

// 初始化并校验环境变量
func init() {
	var err error
	runtimeEnv, err = bootstrap.LoadStartupRuntimeEnv()
	if err != nil {
		log.Fatalf("validate environment failed: %v", err)
	}
}

func main() {
	// 初始化基础组件
	bootstrapApp := bootstrap.New(runtimeEnv)
	bootstrapApp.InitConfig()      // 初始化配置
	bootstrapApp.InitLogger()      // 初始化日志
	bootstrapApp.InitIDGenerator() // 初始化 ID 生成器
	bootstrapApp.InitCron()        // 初始化定时任务
	bootstrapApp.InitMySQL()       // 创建 MySQL 客户端
	bootstrapApp.InitRedis()       // 创建 Redis 客户端
	bootstrapApp.InitKeyManager()  // 创建密钥管理器

	// 创建应用实例
	application := app.NewApp(bootstrapApp)

	// 运行应用并处理优雅关闭
	application.RunWithGracefulShutdown()
}

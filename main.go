package main

import (
	"log"

	"meta-api/app"
	"meta-api/bootstrap"
)

// 生产环境的配置来源按敏感度分层：
//  1. config/*.yml：非敏感、可版本化的业务配置，例如日志配置、数据库连接池、限流等。
//  2. docker-compose environment：非敏感运行时变量，例如 APP_ENV、HTTP_PORT、服务地址等。
//  3. Docker secrets：MySQL密码、JWT 签名密钥、OAuth ClientSecret 等核心敏感信息。
//
// 环境变量和 Docker secrets 分开，是为了避免核心敏感信息出现在 docker inspect、进程环境、日志或 shell history 中。
// Docker secrets 会在容器运行时挂载到 /run/secrets 文件系统，不会写入镜像层和容器只读层，仅在容器运行时可见。
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

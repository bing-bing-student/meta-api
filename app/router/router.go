package router

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/dig"

	"meta-api/bootstrap"
)

// SetUpRouter 启动路由
// container 由调用方（app 层）统一构建并传入，避免重复创建容器导致依赖实例发散
func SetUpRouter(bs *bootstrap.Bootstrap, container *dig.Container) (*gin.Engine, error) {
	// 创建路由引擎
	r, err := RouterEngine(bs)
	if err != nil {
		return nil, err
	}

	// 注册路由处理函数
	handlers, err := resolveHandlers(container)
	if err != nil {
		return nil, err
	}

	// 注册管理员路由
	registerAdminRoutes(r.Group("/admin"), handlers)
	// 注册用户路由
	registerUserRoutes(r.Group("/user"), handlers)

	return r, nil
}

package router

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/dig"

	"meta-api/bootstrap"
)

// SetUpRouter 启动路由
// container 由调用方（app 层）统一构建并传入，避免重复创建容器导致依赖实例发散
func SetUpRouter(bs *bootstrap.Bootstrap, container *dig.Container) (*gin.Engine, error) {
	r, err := RouterEngine(bs)
	if err != nil {
		return nil, err
	}

	handlers, err := resolveHandlers(container)
	if err != nil {
		return nil, err
	}

	registerAdminRoutes(r.Group("/admin"), handlers)
	registerUserRoutes(r.Group("/user"), handlers)

	return r, nil
}

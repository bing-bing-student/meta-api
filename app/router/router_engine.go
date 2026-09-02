package router

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"

	"meta-api/bootstrap"
	"meta-api/common/middlewares"
)

func RouterEngine(bs *bootstrap.Bootstrap) (*gin.Engine, error) {
	gin.SetMode(gin.ReleaseMode)

	r := gin.New()
	if err := r.SetTrustedProxies([]string{"172.16.0.0/12"}); err != nil {
		return nil, fmt.Errorf("set trusted proxies: %w", err)
	}
	// 仅当 TCP 对端位于可信 Docker 代理网段时，Gin 才会解析这些代理头。
	// 不使用 TrustedPlatform：该配置会无条件信任指定请求头，绕过上面的网段校验。
	// 保留线上 Nginx 已规范化的 X-Client-IP，但它现在和其他代理头一样，
	// 只有在 TCP 对端属于可信代理网段时才会被采用。
	r.RemoteIPHeaders = []string{"X-Client-IP", "X-Forwarded-For", "X-Real-IP"}

	r.Use(
		middlewares.TimeoutMiddleware(3*time.Second, timeoutOverrides()...),
		middlewares.GinLogger(bs.Logger),
		middlewares.GinRecovery(bs.Logger, true),
	)

	return r, nil
}

func timeoutOverrides() []middlewares.TimeoutOverride {
	return []middlewares.TimeoutOverride{
		{Prefix: "/user/auth/oauth/", Timeout: 30 * time.Second},
		{Prefix: "/admin/auth/article/update", Timeout: 30 * time.Second},
		{Prefix: "/admin/auth/article/delete", Timeout: 30 * time.Second},
		{Prefix: "/admin/auth/article/image/upload", Timeout: 10 * time.Second},
		{Prefix: "/admin/auth/tag/update", Timeout: 30 * time.Second},
		{Prefix: "/user/bug-feedback", Timeout: 10 * time.Second},
	}
}

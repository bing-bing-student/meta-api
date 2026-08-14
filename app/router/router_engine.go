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
	r.TrustedPlatform = "X-Client-IP"

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

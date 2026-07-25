package middlewares

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"meta-api/common/codes"
	"meta-api/common/types"
)

type TimeoutOverride struct {
	Prefix  string
	Timeout time.Duration
}

// TimeoutMiddleware 超时中间件
func TimeoutMiddleware(timeout time.Duration, overrides ...TimeoutOverride) gin.HandlerFunc {
	return func(c *gin.Context) {
		requestTimeout := timeoutForPath(c.Request.URL.Path, timeout, overrides)
		ctx, cancel := context.WithTimeout(c.Request.Context(), requestTimeout)
		defer cancel()

		c.Request = c.Request.WithContext(ctx)
		c.Next()

		if errors.Is(ctx.Err(), context.DeadlineExceeded) && !c.Writer.Written() {
			c.AbortWithStatusJSON(http.StatusOK, types.Response{Code: codes.RequestTimeout, Message: "请求超时", Data: nil})
		}
	}
}

func timeoutForPath(path string, fallback time.Duration, overrides []TimeoutOverride) time.Duration {
	for _, override := range overrides {
		if override.Prefix == "" || override.Timeout <= 0 {
			continue
		}
		if strings.HasPrefix(path, override.Prefix) {
			return override.Timeout
		}
	}
	return fallback
}

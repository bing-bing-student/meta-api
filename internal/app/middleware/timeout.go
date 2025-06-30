package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"meta-api/internal/common/codes"
	"meta-api/internal/common/types"
)

// TimeoutMiddleware 超时中间件
func TimeoutMiddleware(timeout time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		defer cancel()

		c.Request = c.Request.WithContext(ctx)

		// 等待请求处理完成或超时
		done := make(chan bool)
		go func() {
			c.Next()
			done <- true
		}()

		select {
		case <-done:
		case <-ctx.Done():
			c.JSON(http.StatusOK, types.Response{Code: codes.RequestTimeout, Message: "请求超时", Data: nil})
			c.Abort()
		}
	}
}

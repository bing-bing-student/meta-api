package middlewares

import (
	"errors"
	"net"
	"net/http"
	"net/http/httputil"
	"runtime/debug"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"api-server/common/global"
)

// GinRecovery recover掉项目可能出现的panic
func GinRecovery(stack bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				var brokenPipe bool
				if e, ok := err.(error); ok {
					var netErr net.Error
					if errors.As(e, &netErr) {
						if strings.Contains(strings.ToLower(e.Error()), "broken pipe") ||
							strings.Contains(strings.ToLower(e.Error()), "connection reset by peer") {
							brokenPipe = true
						}
					}
				}

				httpRequest, _ := httputil.DumpRequest(c.Request, false)
				if brokenPipe {
					global.Logger.Error(c.Request.URL.Path,
						zap.Any("code", err),
						zap.String("request", string(httpRequest)),
					)
					_ = c.Error(err.(error))
					c.Abort()
					return
				}

				if stack {
					global.Logger.Error("[Recovery from panic]",
						zap.Any("code", err),
						zap.String("request", string(httpRequest)),
						zap.String("stack", string(debug.Stack())),
					)
				} else {
					global.Logger.Error("[Recovery from panic]",
						zap.Any("code", err),
						zap.String("request", string(httpRequest)),
					)
				}
				c.AbortWithStatus(http.StatusInternalServerError)
			}
		}()
		c.Next()
	}
}

package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"meta-api/internal/common/codes"
	"meta-api/internal/common/types"
	"meta-api/internal/common/utils"
)

// JWT 定义中间件, 进行用户权限校验
func JWT() gin.HandlerFunc {
	return func(c *gin.Context) {
		var err error
		token := c.GetHeader("Authorization")
		if token == "" {
			c.JSON(http.StatusOK, types.Response{
				Code:    codes.Unauthorized,
				Message: "需要授权令牌",
				Data:    nil,
			})
			c.Abort()
			return
		}
		// 去掉Bearer前缀
		token = strings.TrimPrefix(token, "Bearer ")
		if token != "" {
			if _, err = utils.ParseToken(token); err == nil {
				c.Next()
				return
			} else if strings.Contains(err.Error(), "TokenExpired") {
				// 返回过期的业务状态码, 让前端去拿RefreshToken过来, 去请求/refresh-token接口
				c.JSON(http.StatusOK, types.Response{
					Code:    codes.TokenExpired,
					Message: "Token已过期",
					Data:    nil,
				})
				c.Abort()
				return
			}

			// AssetToken解析失败
			c.JSON(http.StatusOK, types.Response{
				Code:    codes.AuthFailed,
				Message: "无效的Token",
				Data:    nil,
			})
			c.Abort()
			return
		}
		return
	}
}

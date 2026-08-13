package middlewares

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"meta-api/common/codes"
	"meta-api/common/types"
	"meta-api/common/utils"
)

// JWT 定义中间件, 进行用户权限校验
func JWT() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从 Cookie 读取 access_token，不再从 Authorization 头读取
		token, err := c.Cookie(utils.AccessTokenCookie)
		if err != nil || token == "" {
			c.JSON(http.StatusOK, types.Response{
				Code:    codes.Unauthorized,
				Message: "需要授权令牌",
				Data:    nil,
			})
			c.Abort()
			return
		}

		claims, err := utils.ParseAccessToken(token)
		if err != nil {
			if strings.Contains(err.Error(), "TokenExpired") {
				c.JSON(http.StatusOK, types.Response{
					Code:    codes.TokenExpired,
					Message: "Token 已过期",
					Data:    nil,
				})
				c.Abort()
				return
			}

			c.JSON(http.StatusOK, types.Response{
				Code:    codes.Unauthorized,
				Message: "无效的 Token",
				Data:    nil,
			})
			c.Abort()
			return
		}

		c.Set("userID", claims.UserID)
		c.Next()
	}
}

package middlewares

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"meta-api/common/codes"
	"meta-api/common/types"
	"meta-api/common/utils"
)

const (
	CommentUserIDKey             = "commentUserID"
	CommentUserSessionVersionKey = "commentUserSessionVersion"
)

func CommentUserJWT() gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := c.Cookie(utils.CommentAccessTokenCookie)
		if err != nil || token == "" {
			c.JSON(http.StatusOK, types.Response{
				Code:    codes.Unauthorized,
				Message: "登录后才能评论",
				Data:    nil,
			})
			c.Abort()
			return
		}

		claims, err := utils.ParseCommentUserToken(token)
		if err != nil {
			if strings.Contains(err.Error(), "TokenExpired") {
				c.JSON(http.StatusOK, types.Response{
					Code:    codes.TokenExpired,
					Message: "登录状态已过期，请重新登录",
					Data:    nil,
				})
				c.Abort()
				return
			}

			utils.ClearCommentAuthCookie(c)
			c.JSON(http.StatusOK, types.Response{
				Code:    codes.Unauthorized,
				Message: "无效的登录状态",
				Data:    nil,
			})
			c.Abort()
			return
		}

		c.Set(CommentUserIDKey, claims.UserID)
		c.Set(CommentUserSessionVersionKey, claims.SessionVersion)
		c.Next()
	}
}

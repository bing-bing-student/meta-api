package admin

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"meta-api/common/codes"
	"meta-api/common/types"
	"meta-api/common/utils"
)

// RefreshTokenToLogin 刷新RefreshToken
func (a *adminHandler) RefreshTokenToLogin(c *gin.Context) {
	refreshToken := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
	if refreshToken == "" {
		c.JSON(http.StatusOK, types.Response{Code: codes.Unauthorized, Message: "需要授权令牌", Data: nil})
		return
	}

	// 解析 refreshToken
	userClaims, err := utils.ParseToken(refreshToken)
	if err != nil {
		a.logger.Error("parse refreshToken failed", zap.Error(err))
		codeInfo := 0
		messageInfo := ""
		if errors.Is(err, errors.New("TokenExpired")) {
			codeInfo = codes.TokenExpired
			messageInfo = "Token已过期"
		} else {
			codeInfo = codes.AuthFailed // 4011: Token 无效
			messageInfo = "无效的Token"
		}

		c.JSON(http.StatusOK, types.Response{Code: codeInfo, Message: messageInfo, Data: nil})
		return
	}

	// 生成新访问令牌和刷新令牌
	doubleToken, err := a.service.GenerateTokenService(userClaims)
	if err != nil {
		c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "生成Token失败", Data: nil})
		return
	}

	// 返回新生成的访问令牌和刷新令牌
	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "",
		Data: map[string]string{
			"userID":       userClaims.UserID,
			"accessToken":  doubleToken.AccessToken,
			"refreshToken": doubleToken.RefreshToken,
		},
	})
}

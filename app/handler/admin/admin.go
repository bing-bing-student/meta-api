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

// RefreshToken 刷新RefreshToken
func (a *adminHandler) RefreshToken(c *gin.Context) {
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
	doubleToken, err := a.service.GenerateToken(userClaims)
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

// SendSMSCode 发送短信验证码
func (a *adminHandler) SendSMSCode(c *gin.Context) {
	//ctx := c.Request.Context()

	//request := new(types.SendSMSCodeRequest)
	//if err := c.ShouldBind(request); err != nil {
	//	a.logger.Error("parameter binding error", zap.Error(err))
	//	c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
	//	return
	//}
	//if err := a.service.SendSMSCode(ctx, request); err != nil {
	//	c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: err.Error(), Data: nil})
	//	return
	//}

	c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "短信发送服务错误", Data: nil})
}

// SMSCodeLogin 短信验证码登录
func (a *adminHandler) SMSCodeLogin(c *gin.Context) {
	ctx := c.Request.Context()

	request := new(types.SMSCodeLoginRequest)
	if err := c.ShouldBind(request); err != nil {
		a.logger.Error("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
	}

	response, err := a.service.SMSCodeLogin(ctx, request)
	if err != nil {
		c.JSON(http.StatusOK, types.Response{Code: codes.AuthFailed, Message: err.Error(), Data: nil})
		return
	}

	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: response})
}

// AccountLogin 账号密码登录
func (a *adminHandler) AccountLogin(c *gin.Context) {
	ctx := c.Request.Context()

	request := new(types.AccountLoginRequest)
	if err := c.ShouldBind(request); err != nil {
		a.logger.Error("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
		return
	}

	response, err := a.service.AccountLogin(ctx, request)
	if err != nil {
		c.JSON(http.StatusOK, types.Response{Code: codes.AuthFailed, Message: err.Error(), Data: nil})
		return
	}

	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: response})
}

// BindDynamicCode 绑定动态码
func (a *adminHandler) BindDynamicCode(c *gin.Context) {
	ctx := c.Request.Context()

	request := new(types.BindDynamicCodeRequest)
	if err := c.ShouldBind(request); err != nil {
		a.logger.Error("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
		return
	}

	response, err := a.service.BindDynamicCode(ctx, request)
	if err != nil {
		c.JSON(http.StatusOK, types.Response{Code: codes.AuthFailed, Message: err.Error(), Data: nil})
	}

	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: response})
}

// VerifyDynamicCode 验证动态码
func (a *adminHandler) VerifyDynamicCode(c *gin.Context) {
	ctx := c.Request.Context()

	request := new(types.VerifyDynamicCodeRequest)
	if err := c.ShouldBind(request); err != nil {
		a.logger.Error("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: err.Error(), Data: nil})
	}

	response, err := a.service.VerifyDynamicCode(ctx, request)
	if err != nil {
		c.JSON(http.StatusOK, types.Response{Code: codes.AuthFailed, Message: "非法的请求参数", Data: nil})
		return
	}

	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: response})
}

// AdminUpdateAboutMe 修改关于我
func (a *adminHandler) AdminUpdateAboutMe(c *gin.Context) {
	ctx := c.Request.Context()

	request := new(types.UpdateAboutMeRequest)
	if err := c.ShouldBind(request); err != nil {
		a.logger.Error("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
		return
	}

	if err := a.service.AdminUpdateAboutMe(ctx, request); err != nil {
		c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "更新失败", Data: nil})
		return
	}
	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: nil})
}

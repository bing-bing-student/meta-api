package admin

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"meta-api/common/codes"
	"meta-api/common/ratelimit"
	"meta-api/common/types"
	"meta-api/common/utils"
)

// writeRateLimited 将限流错误写成统一业务响应。
func writeRateLimited(c *gin.Context, err error) bool {
	limited, ok := ratelimit.AsLimited(err)
	if !ok {
		return false
	}
	c.JSON(http.StatusOK, types.Response{
		Code:    codes.TooManyRequests,
		Message: limited.Error(),
		Data:    types.RetryAfterResponse{RetryAfter: limited.RetryAfterSeconds()},
	})
	return true
}

// RefreshToken 刷新RefreshToken
func (a *adminHandler) RefreshToken(c *gin.Context) {
	// 从 Cookie 读取 refresh_token，不再从 Authorization 头读取
	refreshToken, err := c.Cookie(utils.RefreshTokenCookie)
	if err != nil || refreshToken == "" {
		c.JSON(http.StatusOK, types.Response{Code: codes.Unauthorized, Message: "需要授权令牌", Data: nil})
		return
	}

	// 解析 refreshToken
	userClaims, err := utils.ParseToken(refreshToken)
	if err != nil {
		a.logger.Error("parse refreshToken failed", zap.Error(err))
		// refresh token 无效 / 过期，清除 Cookie 并返回 4010，前端跳转登录页
		utils.ClearAuthCookies(c)
		c.JSON(http.StatusOK, types.Response{Code: codes.Unauthorized, Message: "无效的 Token", Data: nil})
		return
	}

	// 生成新访问令牌和刷新令牌（滚动刷新）
	doubleToken, err := a.service.GenerateToken(userClaims)
	if err != nil {
		c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "生成 Token 失败", Data: nil})
		return
	}

	// 通过 Set-Cookie 下发新的 access_token 和 refresh_token，响应体不再返回 token
	utils.SetAuthCookies(c, doubleToken.AccessToken, doubleToken.RefreshToken)
	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "",
		Data: map[string]string{"userID": userClaims.UserID},
	})
}

// Logout 登出，清除 access_token 和 refresh_token Cookie
func (a *adminHandler) Logout(c *gin.Context) {
	utils.ClearAuthCookies(c)
	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: nil})
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
		return
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
	request.ClientIP = c.ClientIP()

	response, err := a.service.AccountLogin(ctx, request)
	if err != nil {
		if writeRateLimited(c, err) {
			return
		}
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
	request.ClientIP = c.ClientIP()

	response, err := a.service.BindDynamicCode(ctx, request)
	if err != nil {
		if writeRateLimited(c, err) {
			return
		}
		c.JSON(http.StatusOK, types.Response{Code: codes.AuthFailed, Message: err.Error(), Data: nil})
		return
	}

	// 最终登录成功，通过 Set-Cookie 下发 token
	utils.SetAuthCookies(c, response.AccessToken, response.RefreshToken)
	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: response})
}

// VerifyDynamicCode 验证动态码
func (a *adminHandler) VerifyDynamicCode(c *gin.Context) {
	ctx := c.Request.Context()

	request := new(types.VerifyDynamicCodeRequest)
	if err := c.ShouldBind(request); err != nil {
		a.logger.Error("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: err.Error(), Data: nil})
		return
	}
	request.ClientIP = c.ClientIP()

	response, err := a.service.VerifyDynamicCode(ctx, request)
	if err != nil {
		if writeRateLimited(c, err) {
			return
		}
		c.JSON(http.StatusOK, types.Response{Code: codes.AuthFailed, Message: "非法的请求参数", Data: nil})
		return
	}

	// 最终登录成功，通过 Set-Cookie 下发 token
	utils.SetAuthCookies(c, response.AccessToken, response.RefreshToken)
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

	currentUserID := c.GetString("userID")
	if currentUserID == "" {
		c.JSON(http.StatusOK, types.Response{Code: codes.Unauthorized, Message: "需要授权令牌", Data: nil})
		return
	}
	if request.UserID != "" && request.UserID != currentUserID {
		c.JSON(http.StatusOK, types.Response{Code: codes.Forbidden, Message: "禁止修改其他管理员信息", Data: nil})
		return
	}
	request.UserID = currentUserID

	if err := a.service.AdminUpdateAboutMe(ctx, request); err != nil {
		c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "更新失败", Data: nil})
		return
	}
	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: nil})
}

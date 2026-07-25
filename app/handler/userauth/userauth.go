package userauth

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	userAuthService "meta-api/app/service/userauth"
	"meta-api/common/codes"
	"meta-api/common/types"
	"meta-api/common/utils"
)

func (h *userAuthHandler) OAuthLogin(c *gin.Context) {
	ctx := c.Request.Context()
	uriRequest := new(types.OAuthProviderURIRequest)
	if err := c.ShouldBindUri(uriRequest); err != nil {
		h.logger.Error("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的登录渠道", Data: nil})
		return
	}
	queryRequest := new(types.OAuthLoginQueryRequest)
	if err := c.ShouldBindQuery(queryRequest); err != nil {
		h.logger.Error("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的登录参数", Data: nil})
		return
	}
	request := &types.OAuthLoginRequest{
		Provider: uriRequest.Provider,
		Redirect: queryRequest.Redirect,
	}

	authURL, err := h.service.BuildOAuthLoginURL(ctx, request)
	if err != nil {
		switch {
		case errors.Is(err, userAuthService.ErrOAuthProviderMissing):
			c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "OAuth 登录渠道未配置", Data: nil})
		case errors.Is(err, userAuthService.ErrInvalidOAuthRedirect):
			c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的登录回跳地址", Data: nil})
		default:
			c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "初始化 OAuth 登录失败", Data: nil})
		}
		return
	}

	c.Redirect(http.StatusFound, authURL)
}

func (h *userAuthHandler) OAuthCallback(c *gin.Context) {
	ctx := c.Request.Context()
	uriRequest := new(types.OAuthProviderURIRequest)
	if err := c.ShouldBindUri(uriRequest); err != nil {
		h.logger.Error("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的登录渠道", Data: nil})
		return
	}
	queryRequest := new(types.OAuthCallbackQueryRequest)
	if err := c.ShouldBindQuery(queryRequest); err != nil {
		h.logger.Error("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的登录回调参数", Data: nil})
		return
	}
	request := &types.OAuthCallbackRequest{
		Provider: uriRequest.Provider,
		Code:     queryRequest.Code,
		State:    queryRequest.State,
	}

	response, token, err := h.service.HandleOAuthCallback(ctx, request)
	if err != nil {
		h.logger.Error("oauth callback failed", zap.Error(err))
		c.Redirect(http.StatusFound, "/?comment_auth=failed")
		return
	}

	utils.SetCommentAuthCookie(c, token)
	c.Redirect(http.StatusFound, response.RedirectPath)
}

func (h *userAuthHandler) Me(c *gin.Context) {
	ctx := c.Request.Context()
	c.Header("Cache-Control", "no-store")
	token, err := c.Cookie(utils.CommentAccessTokenCookie)
	if err != nil || token == "" {
		c.JSON(http.StatusOK, types.Response{Code: codes.Unauthorized, Message: "未登录", Data: nil})
		return
	}
	claims, err := utils.ParseCommentUserToken(token)
	if err != nil {
		utils.ClearCommentAuthCookie(c)
		c.JSON(http.StatusOK, types.Response{Code: codes.Unauthorized, Message: "登录状态无效", Data: nil})
		return
	}

	user, err := h.service.GetCurrentUser(ctx, claims.UserID, claims.SessionVersion)
	if err != nil {
		utils.ClearCommentAuthCookie(c)
		c.JSON(http.StatusOK, types.Response{Code: codes.Unauthorized, Message: "登录用户不存在", Data: nil})
		return
	}
	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: user})
}

func (h *userAuthHandler) Logout(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	utils.ClearCommentAuthCookie(c)
	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: nil})
}

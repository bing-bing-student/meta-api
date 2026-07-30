package admin

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"

	adminService "meta-api/app/service/admin"
	"meta-api/common/codes"
	"meta-api/common/ratelimit"
	"meta-api/common/types"
)

const maxBugFeedbackBodyBytes = 4 * 1024 * 1024

// UserGetAboutMe 获取关于我
func (a *adminHandler) UserGetAboutMe(c *gin.Context) {
	ctx := c.Request.Context()

	response, err := a.service.UserGetAboutMe(ctx)
	if err != nil {
		c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "服务内部错误", Data: nil})
		return
	}
	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: response})
}

func (a *adminHandler) UserSubmitBugFeedback(c *gin.Context) {
	c.Header("Cache-Control", "no-store, private")
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBugFeedbackBodyBytes)

	ctx := c.Request.Context()
	request := new(types.SubmitBugFeedbackRequest)
	if err := c.ShouldBindJSON(request); err != nil {
		a.logger.Warn("bug feedback parameter binding error", zap.Error(err))
		c.JSON(http.StatusBadRequest, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
		return
	}
	request.ClientIP = c.ClientIP()

	if err := a.service.UserSubmitBugFeedback(ctx, request); err != nil {
		if limited, ok := ratelimit.AsLimited(err); ok {
			c.JSON(http.StatusTooManyRequests, types.Response{
				Code:    codes.TooManyRequests,
				Message: limited.Error(),
				Data:    types.RetryAfterResponse{RetryAfter: limited.RetryAfterSeconds()},
			})
			return
		}
		switch {
		case errors.Is(err, adminService.ErrBugFeedbackInvalid):
			c.JSON(http.StatusBadRequest, types.Response{Code: codes.BadRequest, Message: err.Error(), Data: nil})
		case errors.Is(err, adminService.ErrBugFeedbackSMTPNotConfigured):
			c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "反馈邮件服务未配置", Data: nil})
		case errors.Is(err, adminService.ErrBugFeedbackRecipientNotConfigured):
			c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "未找到反馈收件邮箱", Data: nil})
		default:
			a.logger.Error("bug feedback submit failed", zap.Error(err))
			c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "反馈提交失败", Data: nil})
		}
		return
	}

	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: nil})
}

func (a *adminHandler) AdminGetUserList(c *gin.Context) {
	ctx := c.Request.Context()
	request := new(types.AdminGetUserListRequest)
	if err := c.ShouldBindQuery(request); err != nil {
		a.logger.Warn("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
		return
	}

	response, err := a.service.AdminGetUserList(ctx, request)
	if err != nil {
		c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "获取用户列表失败", Data: nil})
		return
	}
	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: response})
}

func (a *adminHandler) AdminUpdateUserCommentPermission(c *gin.Context) {
	ctx := c.Request.Context()
	request := new(types.AdminUpdateUserCommentPermissionRequest)
	if err := c.ShouldBindJSON(request); err != nil {
		a.logger.Warn("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
		return
	}

	if err := a.service.AdminUpdateUserCommentPermission(ctx, request); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusOK, types.Response{Code: codes.NotFound, Message: "用户不存在", Data: nil})
			return
		}
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "更新评论权限失败", Data: nil})
		return
	}
	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: nil})
}

func (a *adminHandler) AdminForceUserLogout(c *gin.Context) {
	ctx := c.Request.Context()
	request := new(types.AdminForceUserLogoutRequest)
	if err := c.ShouldBindJSON(request); err != nil {
		a.logger.Warn("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
		return
	}

	if err := a.service.AdminForceUserLogout(ctx, request); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusOK, types.Response{Code: codes.NotFound, Message: "用户不存在", Data: nil})
			return
		}
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "强制下线失败", Data: nil})
		return
	}
	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: nil})
}

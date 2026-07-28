package admin

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"meta-api/common/codes"
	"meta-api/common/types"
)

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

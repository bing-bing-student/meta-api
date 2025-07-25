package link

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"meta-api/common/codes"
	"meta-api/common/types"
)

// AdminGetLinkList 获取友链列表
func (l *linkHandler) AdminGetLinkList(c *gin.Context) {
	ctx := c.Request.Context()

	response, err := l.service.AdminGetLinkList(ctx)
	if err != nil {
		c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "获取友链列表失败", Data: nil})
		return
	}
	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: response})
}

// AdminAddLink 添加友链
func (l *linkHandler) AdminAddLink(c *gin.Context) {
	ctx := c.Request.Context()

	request := &types.AdminAddLinkRequest{}
	if err := c.ShouldBind(request); err != nil {
		l.logger.Error("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
		return
	}

	if err := l.service.AdminAddLink(ctx, request); err != nil {
		c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "添加友链失败", Data: nil})
		return
	}
	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: nil})
}

// AdminUpdateLink 修改友链
func (l *linkHandler) AdminUpdateLink(c *gin.Context) {
	ctx := c.Request.Context()

	request := &types.AdminUpdateLinkRequest{}
	if err := c.ShouldBind(request); err != nil {
		l.logger.Error("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
		return
	}

	if err := l.service.AdminUpdateLink(ctx, request); err != nil {
		c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "更新友链失败", Data: nil})
		return
	}
	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: nil})
}

// AdminDeleteLink 删除友链
func (l *linkHandler) AdminDeleteLink(c *gin.Context) {
	ctx := c.Request.Context()

	request := &types.AdminDeleteLinkRequest{}
	if err := c.ShouldBind(request); err != nil {
		l.logger.Error("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
		return
	}

	if err := l.service.AdminDeleteLink(ctx, request); err != nil {
		c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "删除友链失败", Data: nil})
		return
	}
	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: nil})
}

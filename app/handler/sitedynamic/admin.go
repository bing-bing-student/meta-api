package sitedynamic

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"meta-api/common/codes"
	"meta-api/common/types"
)

func (h *siteDynamicHandler) AdminGetSiteDynamicList(c *gin.Context) {
	ctx := c.Request.Context()
	response, err := h.service.AdminGetSiteDynamicList(ctx)
	if err != nil {
		c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "获取站点动态失败", Data: nil})
		return
	}
	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: response})
}

func (h *siteDynamicHandler) AdminAddSiteDynamic(c *gin.Context) {
	ctx := c.Request.Context()
	request := new(types.AdminAddSiteDynamicRequest)
	if err := c.ShouldBindJSON(request); err != nil {
		h.logger.Warn("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
		return
	}

	if err := h.service.AdminAddSiteDynamic(ctx, request); err != nil {
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "添加站点动态失败", Data: nil})
		return
	}
	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: nil})
}

func (h *siteDynamicHandler) AdminUpdateSiteDynamic(c *gin.Context) {
	ctx := c.Request.Context()
	request := new(types.AdminUpdateSiteDynamicRequest)
	if err := c.ShouldBindJSON(request); err != nil {
		h.logger.Warn("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
		return
	}

	if err := h.service.AdminUpdateSiteDynamic(ctx, request); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusOK, types.Response{Code: codes.NotFound, Message: "站点动态不存在", Data: nil})
			return
		}
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "更新站点动态失败", Data: nil})
		return
	}
	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: nil})
}

func (h *siteDynamicHandler) AdminReorderSiteDynamics(c *gin.Context) {
	ctx := c.Request.Context()
	request := new(types.AdminReorderSiteDynamicRequest)
	if err := c.ShouldBindJSON(request); err != nil {
		h.logger.Warn("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
		return
	}

	if err := h.service.AdminReorderSiteDynamics(ctx, request); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusOK, types.Response{Code: codes.NotFound, Message: "站点动态不存在", Data: nil})
			return
		}
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "保存站点动态排序失败", Data: nil})
		return
	}
	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: nil})
}

func (h *siteDynamicHandler) AdminDeleteSiteDynamic(c *gin.Context) {
	ctx := c.Request.Context()
	request := new(types.AdminDeleteSiteDynamicRequest)
	if err := c.ShouldBindJSON(request); err != nil {
		h.logger.Warn("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
		return
	}

	if err := h.service.AdminDeleteSiteDynamic(ctx, request); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusOK, types.Response{Code: codes.NotFound, Message: "站点动态不存在", Data: nil})
			return
		}
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "删除站点动态失败", Data: nil})
		return
	}
	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: nil})
}

package admin

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"meta-api/common/codes"
	"meta-api/common/types"
)

// UserGetAboutMe 获取关于我
func (a *adminHandler) UserGetAboutMe(c *gin.Context) {
	ctx := c.Request.Context()

	response, err := a.service.UserGetAboutMe(ctx)
	if err != nil {
		a.logger.Error("failed to get admin info", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "服务内部错误", Data: nil})
		return
	}
	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: response})
}

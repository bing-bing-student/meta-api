package admin

import (
	"net/http"

	"github.com/gin-gonic/gin"

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

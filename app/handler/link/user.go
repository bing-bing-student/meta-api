package link

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"meta-api/common/codes"
	"meta-api/common/types"
)

// UserGetLinkList 获取链接列表
func (l *linkHandler) UserGetLinkList(c *gin.Context) {
	ctx := c.Request.Context()

	response, err := l.service.UserGetLinkList(ctx)
	if err != nil {
		l.logger.Error("failed to get link list", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "获取友链列表失败", Data: nil})
		return
	}
	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: response})
}

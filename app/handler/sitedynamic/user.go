package sitedynamic

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"meta-api/common/codes"
	"meta-api/common/types"
)

func (h *siteDynamicHandler) UserGetSiteDynamicList(c *gin.Context) {
	ctx := c.Request.Context()
	response, err := h.service.UserGetSiteDynamicList(ctx)
	if err != nil {
		c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "获取站点动态失败", Data: nil})
		return
	}
	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: response})
}

package article

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"meta-api/internal/common/codes"
	"meta-api/internal/common/types"
)

// AdminGetArticleList 管理员获取文章列表
func (h *Handler) AdminGetArticleList(c *gin.Context) {
	req := new(types.AdminGetArticleListRequest)
	if err := c.ShouldBind(req); err != nil {
		h.logger.Error("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
		return
	}

	resp := new(types.AdminGetArticleListResponse)
	if err := h.service.AdminGetArticleListService(req, resp); err != nil {
		h.logger.Error("failed to get article list", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "获取文章列表失败", Data: nil})
		return
	}
	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: resp})
}

package tag

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"meta-api/common/codes"
	"meta-api/common/types"
)

// UserGetTagList 获取标签列表
func (t *tagHandler) UserGetTagList(c *gin.Context) {
	ctx := c.Request.Context()

	response, err := t.service.UserGetTagList(ctx)
	if err != nil {
		t.logger.Error("failed to get tag list from redis", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "获取标签列表失败", Data: nil})
		return
	}
	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: response})
}

// UserGetArticleListByTag 获取标签下的文章列表
func (t *tagHandler) UserGetArticleListByTag(c *gin.Context) {
	ctx := c.Request.Context()

	request := &types.UserGetArticleListByTagRequest{}
	if err := c.ShouldBind(request); err != nil {
		t.logger.Error("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
		return
	}

	response, err := t.service.UserGetArticleListByTag(ctx, request)
	if err != nil {
		t.logger.Error("failed to get article list by tag", zap.Error(err))
		if err.Error() == "not found tagName" {
			c.JSON(http.StatusOK, types.Response{Code: codes.NotFound, Message: "文章标签不存在", Data: nil})
			return
		}
		c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "获取文章列表失败", Data: nil})
		return
	}
	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: response})
}

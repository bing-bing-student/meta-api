package tag

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"meta-api/common/codes"
	"meta-api/common/types"
)

// AdminGetTagList 获取标签列表
func (t *tagHandler) AdminGetTagList(c *gin.Context) {
	ctx := c.Request.Context()

	response, err := t.service.AdminGetTagList(ctx)
	if err != nil {
		t.logger.Error("failed to get tag list from redis", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "获取标签列表失败", Data: nil})
		return
	}
	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: response})
}

// AdminGetArticleListByTag 获取标签下的文章列表
func (t *tagHandler) AdminGetArticleListByTag(c *gin.Context) {
	ctx := c.Request.Context()

	request := &types.AdminGetArticleListByTagRequest{}
	if err := c.ShouldBind(request); err != nil {
		t.logger.Error("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
		return
	}

	resp := new(types.AdminGetArticleListByTagResponse)
	response, err := t.service.AdminGetArticleListByTag(ctx, request)
	if err != nil {
		t.logger.Error("failed to get article list by tag from redis", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "获取文章列表失败", Data: nil})
		return
	}
	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: response})
}

// AdminUpdateTag 更新标签
func (t *tagHandler) AdminUpdateTag(c *gin.Context) {
	req := new(types.AdminUpdateTagRequest)
	if err := c.ShouldBind(req); err != nil {
		t.logger.Error("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
		return
	}
	if req.NewTagName == req.OldTagName {
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "新标签名和旧标签名不能相同", Data: nil})
		return
	}

	if err := t.service.UpdateTagService(req); err != nil {
		t.logger.Error("failed to update tag", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "更新失败", Data: nil})
		return
	}
	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: nil})
}

package article

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"meta-api/common/codes"
	"meta-api/common/constants"
	"meta-api/common/types"
)

// AdminGetArticleList 管理员获取文章列表
func (a *articleHandler) AdminGetArticleList(c *gin.Context) {
	ctx := c.Request.Context()
	request := new(types.AdminGetArticleListRequest)
	if err := c.ShouldBind(request); err != nil {
		a.logger.Error("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
		return
	}

	response, err := a.service.AdminGetArticleList(ctx, request)
	if err != nil {
		a.logger.Error("failed to get article list", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "获取文章列表失败", Data: nil})
		return
	}

	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: response})
}

// AdminGetArticleDetail 获取文章详情
func (a *articleHandler) AdminGetArticleDetail(c *gin.Context) {
	ctx := c.Request.Context()
	request := new(types.AdminGetArticleDetailRequest)
	if err := c.ShouldBind(request); err != nil {
		a.logger.Error("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
		return
	}

	response, err := a.service.AdminGetArticleDetail(ctx, request)
	if err != nil {
		a.logger.Error("failed to get article detail", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "获取文章详情失败", Data: nil})
		return
	}

	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: response})
}

// AdminAddArticle 添加文章
func (a *articleHandler) AdminAddArticle(c *gin.Context) {
	ctx := c.Request.Context()
	request := new(types.AdminAddArticleRequest)
	if err := c.ShouldBind(request); err != nil {
		a.logger.Error("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
		return
	}
	if int64(len(request.Content)) > constants.MaxFileSize {
		a.logger.Error("Article content exceeds 64KB")
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "文章内容超过64KB", Data: nil})
		return
	}

	if err := a.service.AdminAddArticle(ctx, request); err != nil {
		a.logger.Error("failed to add article to mysql", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "添加文章失败", Data: nil})
		return
	}

	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: nil})
}

// AdminUpdateArticle 修改文章
func (a *articleHandler) AdminUpdateArticle(c *gin.Context) {
	ctx := c.Request.Context()
	request := new(types.AdminUpdateArticleRequest)
	if err := c.ShouldBind(request); err != nil {
		a.logger.Error("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
		return
	}

	if err := a.service.AdminUpdateArticle(ctx, request); err != nil {
		a.logger.Error("failed to update article in mysql", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "更新文章失败", Data: nil})
		return
	}

	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: nil})
}

// AdminDeleteArticle 删除文章
func (a *articleHandler) AdminDeleteArticle(c *gin.Context) {
	ctx := c.Request.Context()
	request := new(types.AdminDeleteArticleRequest)
	if err := c.ShouldBind(request); err != nil {
		a.logger.Error("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
		return
	}

	if err := a.service.AdminDeleteArticle(ctx, request); err != nil {
		a.logger.Error("failed to delete article in mysql", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "删除文章失败", Data: nil})
		return
	}

	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: nil})
}

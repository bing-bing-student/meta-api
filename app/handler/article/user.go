package article

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"meta-api/common/codes"
	"meta-api/common/types"
)

// UserGetArticleList 获取文章列表
func (a *articleHandler) UserGetArticleList(c *gin.Context) {
	ctx := c.Request.Context()
	request := new(types.UserGetArticleListRequest)
	if err := c.ShouldBind(request); err != nil {
		a.logger.Error("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
		return
	}

	response, err := a.service.UserGetArticleList(ctx, request)
	if err != nil {
		c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "获取文章列表失败", Data: nil})
		return
	}
	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: response})
}

// UserGetArticleDetail 获取文章详情
func (a *articleHandler) UserGetArticleDetail(c *gin.Context) { // ignore_security_alert
	ctx := c.Request.Context()
	request := new(types.UserGetArticleDetailRequest)
	if err := c.ShouldBind(request); err != nil {
		a.logger.Error("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
		return
	}

	response, err := a.service.UserGetArticleDetail(ctx, request) // ignore_security_alert
	if err != nil {
		if strings.Contains(err.Error(), "record not found") {
			c.JSON(http.StatusOK, types.Response{Code: codes.NotFound, Message: "文章不存在", Data: nil})
			return
		}
		c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "获取文章详情失败", Data: nil})
		return
	}
	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: response})
}

// UserSearchArticle 搜索文章
func (a *articleHandler) UserSearchArticle(c *gin.Context) {
	ctx := c.Request.Context()

	request := &types.UserSearchArticleRequest{}
	if err := c.ShouldBind(request); err != nil {
		a.logger.Error("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
		return
	}

	response, err := a.service.UserSearchArticle(ctx, request)
	if err != nil {
		c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "搜索文章失败", Data: nil})
		return
	}
	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: response})
}

// UserGetHotArticle 获取热门文章
func (a *articleHandler) UserGetHotArticle(c *gin.Context) {
	ctx := c.Request.Context()
	response, err := a.service.UserGetHotArticle(ctx)
	if err != nil {
		c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "获取热门文章失败", Data: nil})
		return
	}
	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: response})
}

// UserGetTimeline 获取文章归档
func (a *articleHandler) UserGetTimeline(c *gin.Context) {
	ctx := c.Request.Context()
	response, err := a.service.UserGetTimeline(ctx)
	if err != nil {
		c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "获取归档文章列表失败", Data: nil})
		return
	}
	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: response})
}

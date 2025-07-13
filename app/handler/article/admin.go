package article

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"meta-api/common/codes"
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

	response := new(types.AdminGetArticleListResponse)
	if err := a.service.AdminGetArticleList(ctx, request, response); err != nil {
		a.logger.Error("failed to get article list", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "获取文章列表失败", Data: nil})
		return
	}
	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: response})
}

// AdminGetArticleDetail 获取文章详情
func (a *articleHandler) AdminGetArticleDetail(c *gin.Context) {}

// AdminAddArticle 添加文章
func (a *articleHandler) AdminAddArticle(c *gin.Context) {}

// AdminUpdateArticle 修改文章
func (a *articleHandler) AdminUpdateArticle(c *gin.Context) {}

// AdminDeleteArticle 删除文章
func (a *articleHandler) AdminDeleteArticle(c *gin.Context) {}

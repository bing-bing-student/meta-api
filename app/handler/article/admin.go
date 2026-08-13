package article

import (
	"errors"
	"io"
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
		a.logger.Warn("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
		return
	}

	response, err := a.service.AdminGetArticleList(ctx, request)
	if err != nil {
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
		a.logger.Warn("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
		return
	}

	response, err := a.service.AdminGetArticleDetail(ctx, request)
	if err != nil {
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
		a.logger.Warn("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
		return
	}
	if int64(len(request.Content)) > constants.MaxFileSize {
		a.logger.Warn("article content exceeds 64KB")
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "文章内容超过64KB", Data: nil})
		return
	}

	response, err := a.service.AdminAddArticle(ctx, request)
	if err != nil {
		c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "添加文章失败", Data: nil})
		return
	}

	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: response})
}

// AdminUpdateArticle 修改文章
func (a *articleHandler) AdminUpdateArticle(c *gin.Context) {
	ctx := c.Request.Context()
	request := new(types.AdminUpdateArticleRequest)
	if err := c.ShouldBind(request); err != nil {
		a.logger.Warn("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
		return
	}
	if int64(len(request.Content)) > constants.MaxFileSize {
		a.logger.Warn("article content exceeds 64KB")
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "文章内容超过64KB", Data: nil})
		return
	}

	response, err := a.service.AdminUpdateArticle(ctx, request)
	if err != nil {
		c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "更新文章失败", Data: nil})
		return
	}

	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: response})
}

// AdminDeleteArticle 删除文章
func (a *articleHandler) AdminDeleteArticle(c *gin.Context) {
	ctx := c.Request.Context()
	request := new(types.AdminDeleteArticleRequest)
	if err := c.ShouldBind(request); err != nil {
		a.logger.Warn("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
		return
	}

	if err := a.service.AdminDeleteArticle(ctx, request); err != nil {
		c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "删除文章失败", Data: nil})
		return
	}

	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: nil})
}

// AdminUploadArticleImage 上传文章图片。
func (a *articleHandler) AdminUploadArticleImage(c *gin.Context) {
	ctx := c.Request.Context()
	if err := c.Request.ParseMultipartForm(constants.MaxArticleImageSize); err != nil {
		a.logger.Warn("article image multipart parse error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "图片上传参数无效", Data: nil})
		return
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		a.logger.Warn("article image file missing", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "请选择要上传的图片", Data: nil})
		return
	}
	if fileHeader.Size <= 0 || fileHeader.Size > constants.MaxArticleImageSize {
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "图片大小不能超过10MB", Data: nil})
		return
	}

	file, err := fileHeader.Open()
	if err != nil {
		a.logger.Error("article image open error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "读取图片失败", Data: nil})
		return
	}
	defer file.Close()

	content, err := io.ReadAll(io.LimitReader(file, constants.MaxArticleImageSize+1))
	if err != nil {
		a.logger.Error("article image read error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "读取图片失败", Data: nil})
		return
	}
	if int64(len(content)) > constants.MaxArticleImageSize {
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "图片大小不能超过10MB", Data: nil})
		return
	}

	response, err := a.service.AdminUploadArticleImage(ctx, fileHeader.Filename, fileHeader.Header.Get("Content-Type"), content)
	if err != nil {
		a.logger.Warn("article image upload failed", zap.Error(err))
		message := "图片上传失败"
		if errors.Is(err, ctx.Err()) {
			message = "图片上传已取消"
		}
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: message, Data: nil})
		return
	}

	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: response})
}

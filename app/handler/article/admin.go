package article

import (
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	articleService "meta-api/app/service/article"
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

// AdminGetArticleDraftList 获取文章草稿分页列表。
func (a *articleHandler) AdminGetArticleDraftList(c *gin.Context) {
	ctx := c.Request.Context()
	request := new(types.AdminGetArticleDraftListRequest)
	if err := c.ShouldBind(request); err != nil {
		a.logger.Warn("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
		return
	}

	response, err := a.service.AdminGetArticleDraftList(ctx, request)
	if err != nil {
		a.logger.Error("get article draft list failed", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "获取草稿列表失败", Data: nil})
		return
	}
	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: response})
}

// AdminGetArticleDraftDetail 获取文章草稿详情。
func (a *articleHandler) AdminGetArticleDraftDetail(c *gin.Context) {
	ctx := c.Request.Context()
	request := new(types.AdminGetArticleDraftDetailRequest)
	if err := c.ShouldBind(request); err != nil {
		a.logger.Warn("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
		return
	}

	response, err := a.service.AdminGetArticleDraftDetail(ctx, request)
	if err != nil {
		a.logger.Error("get article draft detail failed", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "获取草稿详情失败", Data: nil})
		return
	}
	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: response})
}

// AdminSaveArticleDraft 新增或更新文章草稿。
func (a *articleHandler) AdminSaveArticleDraft(c *gin.Context) {
	ctx := c.Request.Context()
	request := new(types.AdminSaveArticleDraftRequest)
	if err := c.ShouldBind(request); err != nil {
		a.logger.Warn("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
		return
	}
	if int64(len(request.Content)) > constants.MaxFileSize {
		a.logger.Warn("article draft content exceeds 64KB")
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "草稿内容超过64KB", Data: nil})
		return
	}

	response, err := a.service.AdminSaveArticleDraft(ctx, request)
	if err != nil {
		a.logger.Error("save article draft failed", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "保存草稿失败", Data: nil})
		return
	}
	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: response})
}

// AdminPublishArticleDraft 发布文章草稿。
func (a *articleHandler) AdminPublishArticleDraft(c *gin.Context) {
	ctx := c.Request.Context()
	request := new(types.AdminPublishArticleDraftRequest)
	if err := c.ShouldBind(request); err != nil {
		a.logger.Warn("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
		return
	}
	if int64(len(request.Content)) > constants.MaxFileSize {
		a.logger.Warn("article draft content exceeds 64KB")
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "草稿内容超过64KB", Data: nil})
		return
	}

	response, err := a.service.AdminPublishArticleDraft(ctx, request)
	if err != nil {
		a.logger.Error("publish article draft failed", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "发布草稿失败", Data: nil})
		return
	}
	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: response})
}

// AdminDeleteArticleDraft 删除文章草稿。
func (a *articleHandler) AdminDeleteArticleDraft(c *gin.Context) {
	ctx := c.Request.Context()
	request := new(types.AdminDeleteArticleDraftRequest)
	if err := c.ShouldBind(request); err != nil {
		a.logger.Warn("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
		return
	}

	if err := a.service.AdminDeleteArticleDraft(ctx, request); err != nil {
		a.logger.Error("delete article draft failed", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "删除草稿失败", Data: nil})
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

// AdminScanArticleImages 扫描 COS 图片与文章正文引用关系。
func (a *articleHandler) AdminScanArticleImages(c *gin.Context) {
	ctx := c.Request.Context()
	response, err := a.service.AdminScanArticleImages(ctx)
	if err != nil {
		a.logger.Error("article image scan failed", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "扫描文章图片失败", Data: nil})
		return
	}
	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: response})
}

// AdminGetArticleImageList 获取文章图片分页列表。
func (a *articleHandler) AdminGetArticleImageList(c *gin.Context) {
	ctx := c.Request.Context()
	request := new(types.AdminGetArticleImageListRequest)
	if err := c.ShouldBind(request); err != nil {
		a.logger.Warn("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
		return
	}

	response, err := a.service.AdminGetArticleImageList(ctx, request)
	if err != nil {
		a.logger.Error("get article image list failed", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "获取图片列表失败", Data: nil})
		return
	}
	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: response})
}

// AdminGetArticleImageDetail 获取文章图片详情。
func (a *articleHandler) AdminGetArticleImageDetail(c *gin.Context) {
	ctx := c.Request.Context()
	request := new(types.AdminGetArticleImageDetailRequest)
	if err := c.ShouldBind(request); err != nil {
		a.logger.Warn("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
		return
	}

	response, err := a.service.AdminGetArticleImageDetail(ctx, request)
	if err != nil {
		a.logger.Error("get article image detail failed", zap.Error(err))
		code := codes.InternalServerError
		message := "获取图片详情失败"
		if articleService.IsArticleImageNotFoundError(err) {
			code = codes.NotFound
			message = "图片不存在"
		}
		c.JSON(http.StatusOK, types.Response{Code: code, Message: message, Data: nil})
		return
	}
	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: response})
}

// AdminDeleteArticleImage 删除 COS 图片和本地资产记录。
func (a *articleHandler) AdminDeleteArticleImage(c *gin.Context) {
	ctx := c.Request.Context()
	request := new(types.AdminDeleteArticleImageRequest)
	if err := c.ShouldBind(request); err != nil {
		a.logger.Warn("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
		return
	}

	if err := a.service.AdminDeleteArticleImage(ctx, request); err != nil {
		a.logger.Warn("delete article image failed", zap.Error(err))
		code := codes.InternalServerError
		message := "删除图片失败"
		switch {
		case articleService.IsArticleImageInUseError(err):
			code = codes.BadRequest
			message = "图片仍被文章引用，请确认后强制删除"
		case articleService.IsArticleImageNotFoundError(err):
			code = codes.NotFound
			message = "图片不存在"
		}
		c.JSON(http.StatusOK, types.Response{Code: code, Message: message, Data: nil})
		return
	}

	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: nil})
}

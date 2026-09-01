package comment

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	commentService "meta-api/app/service/comment"
	"meta-api/common/codes"
	"meta-api/common/types"
)

func (h *commentHandler) AdminGetCommentList(c *gin.Context) {
	ctx := c.Request.Context()
	request := new(types.AdminGetCommentListRequest)
	if err := c.ShouldBind(request); err != nil {
		h.logger.Warn("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
		return
	}

	response, err := h.service.AdminGetCommentList(ctx, request)
	if err != nil {
		if errors.Is(err, commentService.ErrInvalidComment) {
			c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
			return
		}
		c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "获取评论列表失败", Data: nil})
		return
	}
	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: response})
}

func (h *commentHandler) AdminGetCommentDetail(c *gin.Context) {
	ctx := c.Request.Context()
	request := new(types.AdminGetCommentDetailRequest)
	if err := c.ShouldBindQuery(request); err != nil {
		h.logger.Warn("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
		return
	}
	response, err := h.service.AdminGetCommentDetail(ctx, request)
	if err != nil {
		if errors.Is(err, commentService.ErrInvalidComment) {
			c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
			return
		}
		if errors.Is(err, commentService.ErrCommentNotFound) {
			c.JSON(http.StatusOK, types.Response{Code: codes.NotFound, Message: "评论不存在", Data: nil})
			return
		}
		c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "获取评论详情失败", Data: nil})
		return
	}
	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: response})
}

func (h *commentHandler) AdminUpdateCommentStatus(c *gin.Context) {
	ctx := c.Request.Context()
	request := new(types.AdminUpdateCommentStatusRequest)
	if err := c.ShouldBind(request); err != nil {
		h.logger.Warn("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
		return
	}

	if err := h.service.AdminUpdateCommentStatus(ctx, request); err != nil {
		if errors.Is(err, commentService.ErrInvalidComment) {
			c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
			return
		}
		if errors.Is(err, commentService.ErrCommentNotFound) {
			c.JSON(http.StatusOK, types.Response{Code: codes.NotFound, Message: "评论不存在", Data: nil})
			return
		}
		c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "更新评论状态失败", Data: nil})
		return
	}
	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: nil})
}

func (h *commentHandler) AdminReviewComment(c *gin.Context) {
	ctx := c.Request.Context()
	request := new(types.AdminReviewCommentRequest)
	if err := c.ShouldBindJSON(request); err != nil {
		h.logger.Warn("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
		return
	}
	request.AdminID = c.GetString("userID")
	if err := h.service.AdminReviewComment(ctx, request); err != nil {
		if errors.Is(err, commentService.ErrInvalidComment) {
			c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
			return
		}
		if errors.Is(err, commentService.ErrCommentNotFound) {
			c.JSON(http.StatusOK, types.Response{Code: codes.NotFound, Message: "评论或审核记录不存在", Data: nil})
			return
		}
		c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "处理评论失败", Data: nil})
		return
	}
	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: nil})
}

func (h *commentHandler) AdminDeleteComment(c *gin.Context) {
	ctx := c.Request.Context()
	request := new(types.AdminDeleteCommentRequest)
	if err := c.ShouldBind(request); err != nil {
		h.logger.Warn("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
		return
	}

	if err := h.service.AdminDeleteComment(ctx, request); err != nil {
		if errors.Is(err, commentService.ErrInvalidComment) {
			c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
			return
		}
		if errors.Is(err, commentService.ErrCommentNotFound) {
			c.JSON(http.StatusOK, types.Response{Code: codes.NotFound, Message: "评论不存在", Data: nil})
			return
		}
		c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "删除评论失败", Data: nil})
		return
	}
	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: nil})
}

func (h *commentHandler) AdminPreviewCommentModeration(c *gin.Context) {
	ctx := c.Request.Context()
	request := new(types.AdminPreviewCommentModerationRequest)
	if err := c.ShouldBind(request); err != nil {
		h.logger.Warn("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
		return
	}
	request.AdminID = c.GetString("userID")

	response, err := h.service.AdminPreviewCommentModeration(ctx, request)
	if err != nil {
		if errors.Is(err, commentService.ErrInvalidComment) {
			c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
			return
		}
		c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "审核模拟失败", Data: nil})
		return
	}
	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: response})
}

func (h *commentHandler) AdminSubmitCommentModerationFeedback(c *gin.Context) {
	ctx := c.Request.Context()
	request := new(types.AdminSubmitCommentModerationFeedbackRequest)
	if err := c.ShouldBindJSON(request); err != nil {
		h.logger.Warn("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
		return
	}
	request.AdminID = c.GetString("userID")
	if err := h.service.AdminSubmitCommentModerationFeedback(ctx, request); err != nil {
		if errors.Is(err, commentService.ErrInvalidComment) {
			c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
			return
		}
		if errors.Is(err, commentService.ErrCommentNotFound) {
			c.JSON(http.StatusOK, types.Response{Code: codes.NotFound, Message: "审核记录不存在", Data: nil})
			return
		}
		c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "保存审核反馈失败", Data: nil})
		return
	}
	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: nil})
}

func (h *commentHandler) AdminGetCommentReportList(c *gin.Context) {
	ctx := c.Request.Context()
	request := new(types.AdminGetCommentReportListRequest)
	if err := c.ShouldBind(request); err != nil {
		h.logger.Warn("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
		return
	}

	response, err := h.service.AdminGetCommentReportList(ctx, request)
	if err != nil {
		if errors.Is(err, commentService.ErrInvalidComment) {
			c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
			return
		}
		c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "获取举报列表失败", Data: nil})
		return
	}
	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: response})
}

func (h *commentHandler) AdminHandleCommentReport(c *gin.Context) {
	ctx := c.Request.Context()
	request := new(types.AdminHandleCommentReportRequest)
	if err := c.ShouldBind(request); err != nil {
		h.logger.Warn("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
		return
	}

	if err := h.service.AdminHandleCommentReport(ctx, request); err != nil {
		if errors.Is(err, commentService.ErrInvalidComment) {
			c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
			return
		}
		if errors.Is(err, commentService.ErrCommentNotFound) {
			c.JSON(http.StatusOK, types.Response{Code: codes.NotFound, Message: "举报或评论不存在", Data: nil})
			return
		}
		c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "处理举报失败", Data: nil})
		return
	}
	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: nil})
}

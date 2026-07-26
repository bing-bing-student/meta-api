package comment

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	commentService "meta-api/app/service/comment"
	"meta-api/common/codes"
	"meta-api/common/middlewares"
	"meta-api/common/ratelimit"
	"meta-api/common/types"
	"meta-api/common/utils"
)

func (h *commentHandler) UserGetCommentList(c *gin.Context) {
	ctx := c.Request.Context()
	request := new(types.UserGetCommentListRequest)
	if err := c.ShouldBind(request); err != nil {
		h.logger.Error("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
		return
	}

	response, err := h.service.UserGetCommentList(ctx, request)
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

func (h *commentHandler) UserGetCommentReplyList(c *gin.Context) {
	ctx := c.Request.Context()
	request := new(types.UserGetCommentReplyListRequest)
	if err := c.ShouldBind(request); err != nil {
		h.logger.Error("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
		return
	}

	response, err := h.service.UserGetCommentReplyList(ctx, request)
	if err != nil {
		switch {
		case errors.Is(err, commentService.ErrInvalidComment):
			c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
		case errors.Is(err, commentService.ErrCommentNotFound):
			c.JSON(http.StatusOK, types.Response{Code: codes.NotFound, Message: "父评论不存在", Data: nil})
		default:
			c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "获取回复列表失败", Data: nil})
		}
		return
	}
	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: response})
}

func (h *commentHandler) UserAddComment(c *gin.Context) {
	ctx := c.Request.Context()
	request := new(types.UserAddCommentRequest)
	if err := c.ShouldBind(request); err != nil {
		h.logger.Error("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
		return
	}
	request.UserID = c.GetString(middlewares.CommentUserIDKey)
	if value, exists := c.Get(middlewares.CommentUserSessionVersionKey); exists {
		if sessionVersion, ok := value.(int64); ok {
			request.SessionVersion = sessionVersion
		}
	}
	request.ClientIP = c.ClientIP()

	response, err := h.service.UserAddComment(ctx, request)
	if err != nil {
		switch {
		case isCommentRateLimited(c, err):
			return
		case errors.Is(err, commentService.ErrCommentSessionInvalid):
			utils.ClearCommentAuthCookie(c)
			c.JSON(http.StatusOK, types.Response{Code: codes.Unauthorized, Message: "登录状态已失效，请重新登录", Data: nil})
		case errors.Is(err, commentService.ErrCommentUnauthorized):
			c.JSON(http.StatusOK, types.Response{Code: codes.Unauthorized, Message: "登录后才能评论", Data: nil})
		case errors.Is(err, commentService.ErrCommentForbidden):
			c.JSON(http.StatusOK, types.Response{Code: codes.Forbidden, Message: "该账号已被限制评论", Data: nil})
		case errors.Is(err, commentService.ErrInvalidComment):
			c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "评论内容无效", Data: nil})
		case errors.Is(err, commentService.ErrCommentNotFound):
			c.JSON(http.StatusOK, types.Response{Code: codes.NotFound, Message: "文章或父评论不存在", Data: nil})
		default:
			c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "提交评论失败", Data: nil})
		}
		return
	}
	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: response})
}

// isCommentRateLimited 将评论提交限流错误写成统一业务响应。
func isCommentRateLimited(c *gin.Context, err error) bool {
	limited, ok := ratelimit.AsLimited(err)
	if !ok {
		return false
	}
	c.JSON(http.StatusOK, types.Response{
		Code:    codes.TooManyRequests,
		Message: limited.Error(),
		Data:    types.RetryAfterResponse{RetryAfter: limited.RetryAfterSeconds()},
	})
	return true
}

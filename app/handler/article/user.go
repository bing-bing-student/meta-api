package article

import (
	"net/http"
	"strings"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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
	session := sessions.Default(c)
	sessionID := session.Get("session_id")
	if sessionID == nil {
		newSessionID := uuid.New().String()
		session.Options(sessions.Options{MaxAge: 86400, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteNoneMode})
		session.Set("session_id", newSessionID)
		if err := session.Save(); err != nil {
			a.logger.Error("failed to save session", zap.Error(err))
			c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "服务器内部错误", Data: nil})
			return
		}
	}

	response, err := a.service.UserGetArticleList(ctx, request)
	if err != nil {
		c.JSON(http.StatusOK, types.Response{Code: codes.InternalServerError, Message: "获取文章列表失败", Data: nil})
		return
	}
	c.JSON(http.StatusOK, types.Response{Code: codes.Success, Message: "", Data: response})
}

// UserGetArticleDetail 获取文章详情
//
// 该接口对所有访客完全开放，不做任何 cookie / 浏览器指纹校验，
// SSR、curl、搜索引擎爬虫均可直接访问，便于 SEO 与服务端渲染场景。
//
// 浏览量去重逻辑保留：cookie 中已有 session_id 的访客复用该值作为锁 key，
// 匿名访客统一走一把全局锁（避免接口被无限刷）。
func (a *articleHandler) UserGetArticleDetail(c *gin.Context) {
	ctx := c.Request.Context()
	request := new(types.UserGetArticleDetailRequest)
	if err := c.ShouldBind(request); err != nil {
		a.logger.Error("parameter binding error", zap.Error(err))
		c.JSON(http.StatusOK, types.Response{Code: codes.BadRequest, Message: "无效的请求参数", Data: nil})
		return
	}

	session := sessions.Default(c)
	userID := ""
	if sid := session.Get("session_id"); sid != nil {
		if s, ok := sid.(string); ok {
			userID = s
		}
	}
	request.UserID = userID

	response, err := a.service.UserGetArticleDetail(ctx, request) // ignore_security_alert // ignore_security_alert // ignore_security_alert
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

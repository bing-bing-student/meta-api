package article

import (
	"github.com/gin-gonic/gin"
)

// UserGetArticleList 获取文章列表
func (a *articleHandler) UserGetArticleList(c *gin.Context) {}

// UserSearchArticle 搜索文章
func (a *articleHandler) UserSearchArticle(c *gin.Context) {}

// UserGetHotArticle 获取热门文章
func (a *articleHandler) UserGetHotArticle(c *gin.Context) {}

// UserGetArticleDetail 获取文章详情
func (a *articleHandler) UserGetArticleDetail(c *gin.Context) {}

// UserGetTimeline 获取文章归档
func (a *articleHandler) UserGetTimeline(c *gin.Context) {}

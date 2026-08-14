package router

import (
	"github.com/gin-gonic/gin"

	"meta-api/common/middlewares"
)

// registerAdminRoutes 注册管理员路由
func registerAdminRoutes(group *gin.RouterGroup, handlers *routeHandlers) {
	registerAdminPublicRoutes(group, handlers)

	authGroup := group.Group("/auth")
	authGroup.Use(middlewares.JWT())
	registerAdminAuthRoutes(authGroup, handlers)
}

// registerAdminPublicRoutes 注册管理员公开路由
func registerAdminPublicRoutes(group *gin.RouterGroup, handlers *routeHandlers) {
	// 登录态维护
	group.POST("/refresh-token", handlers.admin.RefreshToken)
	group.POST("/logout", handlers.admin.Logout)

	// 管理员登录与二次验证
	group.POST("/account-login", handlers.admin.AccountLogin)
	group.POST("/bind-dynamic-code", handlers.admin.BindDynamicCode)
	group.POST("/verify-dynamic-code", handlers.admin.VerifyDynamicCode)
}

// registerAdminAuthRoutes 注册管理员认证路由
func registerAdminAuthRoutes(group *gin.RouterGroup, handlers *routeHandlers) {
	// 文章管理
	group.GET("/article/list", handlers.article.AdminGetArticleList)
	group.GET("/article/detail", handlers.article.AdminGetArticleDetail)
	group.POST("/article/add", handlers.article.AdminAddArticle)
	group.PUT("/article/update", handlers.article.AdminUpdateArticle)
	group.DELETE("/article/delete", handlers.article.AdminDeleteArticle)
	group.POST("/article/image/upload", handlers.article.AdminUploadArticleImage)

	// 标签管理
	group.GET("/tag/list", handlers.tag.AdminGetTagList)
	group.GET("/tag/article-list", handlers.tag.AdminGetArticleListByTag)
	group.PUT("/tag/update", handlers.tag.AdminUpdateTag)

	// 友链管理
	group.GET("/link/list", handlers.link.AdminGetLinkList)
	group.POST("/link/add", handlers.link.AdminAddLink)
	group.PUT("/link/update", handlers.link.AdminUpdateLink)
	group.DELETE("/link/delete", handlers.link.AdminDeleteLink)

	// 评论管理
	group.GET("/comment/list", handlers.comment.AdminGetCommentList)
	group.PUT("/comment/status", handlers.comment.AdminUpdateCommentStatus)
	group.DELETE("/comment/delete", handlers.comment.AdminDeleteComment)
	group.POST("/comment/moderation-preview", handlers.comment.AdminPreviewCommentModeration)
	group.GET("/comment/report-list", handlers.comment.AdminGetCommentReportList)
	group.PUT("/comment/report", handlers.comment.AdminHandleCommentReport)

	// 用户管理
	group.GET("/user/list", handlers.admin.AdminGetUserList)
	group.PUT("/user/comment-permission", handlers.admin.AdminUpdateUserCommentPermission)
	group.PUT("/user/force-logout", handlers.admin.AdminForceUserLogout)

	// 站点资料
	group.PUT("/about-me", handlers.admin.AdminUpdateAboutMe)
}

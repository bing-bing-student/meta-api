package router

import (
	"github.com/gin-gonic/gin"

	"meta-api/common/middlewares"
)

// registerUserRoutes 注册用户路由
func registerUserRoutes(group *gin.RouterGroup, handlers *routeHandlers) {
	// 文章展示
	group.GET("/article/list", handlers.article.UserGetArticleList)
	group.GET("/article/search", handlers.article.UserSearchArticle)
	group.GET("/article/hot", handlers.article.UserGetHotArticle)
	group.GET("/article/detail", handlers.article.UserGetArticleDetail)
	group.GET("/article/timeline", handlers.article.UserGetTimeline)
	group.POST("/article/view-log/:id", handlers.viewLog.PostViewLog)

	// 标签与友链
	group.GET("/tag/list", handlers.tag.UserGetTagList)
	group.GET("/tag/article-list", handlers.tag.UserGetArticleListByTag)
	group.GET("/link", handlers.link.UserGetLinkList)

	// 评论与举报
	group.GET("/comment/list", handlers.comment.UserGetCommentList)
	group.GET("/comment/reply-list", handlers.comment.UserGetCommentReplyList)
	group.POST("/comment/add", middlewares.CommentUserJWT(), handlers.comment.UserAddComment)
	group.POST("/comment/report", middlewares.CommentUserJWT(), handlers.comment.UserReportComment)
	group.POST("/comment/report-status", middlewares.CommentUserJWT(), handlers.comment.UserGetCommentReportStatus)

	// 前台用户认证
	group.GET("/auth/oauth/:provider/login", handlers.userAuth.OAuthLogin)
	group.GET("/auth/oauth/:provider/callback", handlers.userAuth.OAuthCallback)
	group.GET("/auth/me", handlers.userAuth.Me)
	group.POST("/auth/logout", handlers.userAuth.Logout)

	// 站点资料与反馈
	group.GET("/about-me", handlers.admin.UserGetAboutMe)
	group.POST("/bug-feedback", handlers.admin.UserSubmitBugFeedback)

	// JSON 分享风控
	group.POST("/share/precheck", handlers.jsonShare.Precheck)
	group.POST("/share/consume", handlers.jsonShare.Consume)
}

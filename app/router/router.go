package router

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/dig"

	"meta-api/app/handler/admin"
	"meta-api/app/handler/article"
	"meta-api/app/handler/comment"
	"meta-api/app/handler/link"
	"meta-api/app/handler/share"
	"meta-api/app/handler/tag"
	"meta-api/app/handler/userauth"
	"meta-api/app/handler/viewlog"
	"meta-api/bootstrap"
	"meta-api/common/middlewares"
)

// SetUpRouter 启动路由
// container 由调用方（app 层）统一构建并传入，避免重复创建容器导致依赖实例发散
func SetUpRouter(bs *bootstrap.Bootstrap, container *dig.Container) (*gin.Engine, error) {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	logger := bs.Logger

	// 信任 Nginx 代理所在网段的请求
	if err := r.SetTrustedProxies([]string{"172.16.0.0/12"}); err != nil {
		return nil, fmt.Errorf("set trusted proxies: %w", err)
	}
	r.TrustedPlatform = "X-Client-IP"

	// 添加中间件
	r.Use(
		middlewares.TimeoutMiddleware(
			3*time.Second,
			middlewares.TimeoutOverride{Prefix: "/user/auth/oauth/", Timeout: 12 * time.Second},
		),
		middlewares.GinLogger(logger),
		middlewares.GinRecovery(logger, true),
	)

	// 获取 adminHandler 实例
	var adminHandler admin.Handler
	if err := container.Invoke(func(h admin.Handler) { adminHandler = h }); err != nil {
		return nil, fmt.Errorf("resolve admin handler: %w", err)
	}

	// 获取 articleHandler 实例
	var articleHandler article.Handler
	if err := container.Invoke(func(h article.Handler) { articleHandler = h }); err != nil {
		return nil, fmt.Errorf("resolve article handler: %w", err)
	}

	// 获取 tagHandler 实例
	var tagHandler tag.Handler
	if err := container.Invoke(func(h tag.Handler) { tagHandler = h }); err != nil {
		return nil, fmt.Errorf("resolve tag handler: %w", err)
	}

	// 获取 commentHandler 实例
	var commentHandler comment.Handler
	if err := container.Invoke(func(h comment.Handler) { commentHandler = h }); err != nil {
		return nil, fmt.Errorf("resolve comment handler: %w", err)
	}

	// 获取 linkHandler 实例
	var linkHandler link.Handler
	if err := container.Invoke(func(h link.Handler) { linkHandler = h }); err != nil {
		return nil, fmt.Errorf("resolve link handler: %w", err)
	}

	// 获取 viewLogHandler 实例
	var viewLogHandler viewlog.Handler
	if err := container.Invoke(func(h viewlog.Handler) { viewLogHandler = h }); err != nil {
		return nil, fmt.Errorf("resolve view-log handler: %w", err)
	}

	// 获取 shareHandler 实例
	var shareHandler share.Handler
	if err := container.Invoke(func(h share.Handler) { shareHandler = h }); err != nil {
		return nil, fmt.Errorf("resolve share handler: %w", err)
	}

	// 获取 userAuthHandler 实例
	var userAuthHandler userauth.Handler
	if err := container.Invoke(func(h userauth.Handler) { userAuthHandler = h }); err != nil {
		return nil, fmt.Errorf("resolve user auth handler: %w", err)
	}

	// 后台管理路由(不需要JWT验证)
	adminGroup := r.Group("/admin")
	{
		adminGroup.POST("/refresh-token", adminHandler.RefreshToken) // 刷新 RefreshToken
		adminGroup.POST("/logout", adminHandler.Logout)              // 登出，清除Cookie
		// adminGroup.POST("/sms-code", adminHandler.SendSMSCode)                  // 发送短信验证码（已停用）
		adminGroup.POST("/account-login", adminHandler.AccountLogin)            // 账号密码登录
		adminGroup.POST("/bind-dynamic-code", adminHandler.BindDynamicCode)     // 绑定 TOTP 动态码
		adminGroup.POST("/verify-dynamic-code", adminHandler.VerifyDynamicCode) // 验证 TOTP 动态码
	}

	// 后台管理路由(需要JWT验证)
	authAdminGroup := adminGroup.Group("/auth")
	authAdminGroup.Use(middlewares.JWT())
	{
		// 文章管理
		authAdminGroup.GET("/article/list", articleHandler.AdminGetArticleList)
		authAdminGroup.GET("/article/detail", articleHandler.AdminGetArticleDetail)
		authAdminGroup.POST("/article/add", articleHandler.AdminAddArticle)
		authAdminGroup.PUT("/article/update", articleHandler.AdminUpdateArticle)
		authAdminGroup.DELETE("/article/delete", articleHandler.AdminDeleteArticle)

		// 标签管理
		authAdminGroup.GET("/tag/list", tagHandler.AdminGetTagList)
		authAdminGroup.GET("/tag/article-list", tagHandler.AdminGetArticleListByTag)
		authAdminGroup.PUT("/tag/update", tagHandler.AdminUpdateTag)

		// 友链管理
		authAdminGroup.GET("/link/list", linkHandler.AdminGetLinkList)
		authAdminGroup.POST("/link/add", linkHandler.AdminAddLink)
		authAdminGroup.PUT("/link/update", linkHandler.AdminUpdateLink)
		authAdminGroup.DELETE("/link/delete", linkHandler.AdminDeleteLink)

		// 评论管理
		authAdminGroup.GET("/comment/list", commentHandler.AdminGetCommentList)
		authAdminGroup.PUT("/comment/status", commentHandler.AdminUpdateCommentStatus)
		authAdminGroup.DELETE("/comment/delete", commentHandler.AdminDeleteComment)
		authAdminGroup.POST("/comment/moderation-preview", commentHandler.AdminPreviewCommentModeration)
		authAdminGroup.GET("/comment/report-list", commentHandler.AdminGetCommentReportList)
		authAdminGroup.PUT("/comment/report", commentHandler.AdminHandleCommentReport)

		// 用户管理
		authAdminGroup.GET("/user/list", adminHandler.AdminGetUserList)
		authAdminGroup.PUT("/user/comment-permission", adminHandler.AdminUpdateUserCommentPermission)
		authAdminGroup.PUT("/user/force-logout", adminHandler.AdminForceUserLogout)

		// 管理员相关
		authAdminGroup.PUT("/about-me", adminHandler.AdminUpdateAboutMe)
	}

	// 前台展示
	userGroup := r.Group("/user")
	{
		// 文章相关
		userGroup.GET("/article/list", articleHandler.UserGetArticleList)
		userGroup.GET("/article/search", articleHandler.UserSearchArticle)
		userGroup.GET("/article/hot", articleHandler.UserGetHotArticle)
		userGroup.GET("/article/detail", articleHandler.UserGetArticleDetail)
		userGroup.GET("/article/timeline", articleHandler.UserGetTimeline)
		userGroup.POST("/article/view-log/:id", viewLogHandler.PostViewLog)

		// 标签相关
		userGroup.GET("/tag/list", tagHandler.UserGetTagList)
		userGroup.GET("/tag/article-list", tagHandler.UserGetArticleListByTag)

		// 友链相关
		userGroup.GET("/link", linkHandler.UserGetLinkList)

		// 评论相关
		userGroup.GET("/comment/list", commentHandler.UserGetCommentList)
		userGroup.GET("/comment/reply-list", commentHandler.UserGetCommentReplyList)
		userGroup.POST("/comment/add", middlewares.CommentUserJWT(), commentHandler.UserAddComment)
		userGroup.POST("/comment/report", middlewares.CommentUserJWT(), commentHandler.UserReportComment)
		userGroup.POST("/comment/report-status", middlewares.CommentUserJWT(), commentHandler.UserGetCommentReportStatus)

		// 前台登录相关
		userGroup.GET("/auth/oauth/:provider/login", userAuthHandler.OAuthLogin)
		userGroup.GET("/auth/oauth/:provider/callback", userAuthHandler.OAuthCallback)
		userGroup.GET("/auth/me", userAuthHandler.Me)
		userGroup.POST("/auth/logout", userAuthHandler.Logout)

		// 管理员相关
		userGroup.GET("/about-me", adminHandler.UserGetAboutMe)

		// 分享风控守卫（v1）：浏览器→precheck（信封风控签发 token）
		// Nuxt SSR→consume（一次性消费 token，拿到 fingerprint 继续走文件存储）
		userGroup.POST("/share/precheck", shareHandler.Precheck)
		userGroup.POST("/share/consume", shareHandler.Consume)
	}

	return r, nil
}

package router

import (
	"os"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"meta-api/app/di"
	"meta-api/app/handler/admin"
	"meta-api/app/handler/article"
	"meta-api/app/handler/link"
	"meta-api/app/handler/tag"
	"meta-api/bootstrap"
	"meta-api/common/middlewares"
)

// SetUpRouter 启动路由
func SetUpRouter(bs *bootstrap.Bootstrap) *gin.Engine {
	//gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	logger := bs.Logger

	// 禁用代理
	if err := r.SetTrustedProxies(nil); err != nil {
		logger.Error("error set trusted proxy", zap.Error(err))
		return nil
	}

	// 跨域配置(开发阶段使用)
	corsConfig := cors.Config{
		AllowOrigins: []string{"http://localhost:3000", "http://localhost:4000"},
		AllowMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders: []string{
			"Content-Type",
			"Content-Length",
			"Accept-Encoding",
			"Authorization",
			"accept",
			"origin",
			"Cache-Control",
			"x-client-id",
		},
		MaxAge:           24 * time.Hour,
		AllowCredentials: true,
	}

	// 添加中间件
	r.Use(middlewares.TimeoutMiddleware(2*time.Second), cors.New(corsConfig), middlewares.GinLogger(logger), middlewares.GinRecovery(logger, true))
	//r.Use(middlewares.TimeoutMiddleware(2*time.Second), middlewares.GinLogger(loggers), middlewares.GinRecovery(loggers, true))

	container, err := di.BuildContainer(bs)
	if err != nil {
		logger.Error("failed to build container", zap.Error(err))
		return nil
	}

	// 获取 adminHandler 实例
	var adminHandler admin.Handler
	if err = container.Invoke(func(h admin.Handler) { adminHandler = h }); err != nil {
		logger.Error("failed to get admin handler", zap.Error(err))
		return nil
	}

	// 获取 articleHandler 实例
	var articleHandler article.Handler
	if err = container.Invoke(func(h article.Handler) { articleHandler = h }); err != nil {
		logger.Error("failed to get article handler", zap.Error(err))
		return nil
	}

	// 获取 tagHandler 实例
	var tagHandler tag.Handler
	if err = container.Invoke(func(h tag.Handler) { tagHandler = h }); err != nil {
		logger.Error("failed to get tag handler", zap.Error(err))
		return nil
	}

	// 获取 linkHandler 实例
	var linkHandler link.Handler
	if err = container.Invoke(func(h link.Handler) { linkHandler = h }); err != nil {
		logger.Error("failed to get link handler", zap.Error(err))
		return nil
	}

	// 后台管理路由(不需要JWT验证)
	adminGroup := r.Group("/admin")
	{
		adminGroup.POST("/refresh-token", adminHandler.RefreshToken)            // 刷新RefreshToken
		adminGroup.POST("/sms-code", adminHandler.SendSMSCode)                  // 发送短信验证码
		adminGroup.POST("/sms-login", adminHandler.SMSCodeLogin)                // 短信验证码登录
		adminGroup.POST("/account-login", adminHandler.AccountLogin)            // 账号密码登录
		adminGroup.POST("/bind-dynamic-code", adminHandler.BindDynamicCode)     // 绑定TOTP动态码
		adminGroup.POST("/verify-dynamic-code", adminHandler.VerifyDynamicCode) // 验证TOTP动态码
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

		// 管理员相关
		authAdminGroup.PUT("/about-me", adminHandler.AdminUpdateAboutMe)
	}

	// 前台展示
	userGroup := r.Group("/user")
	userGroup.Use(sessions.Sessions("session_id", cookie.NewStore([]byte(os.Getenv("AUTHORIZATION_KEY")), []byte(os.Getenv("ENCRYPTION_KEY")))))
	{
		// 文章相关
		userGroup.GET("/article/list", articleHandler.UserGetArticleList)
		userGroup.GET("/article/search", articleHandler.UserSearchArticle)
		userGroup.GET("/article/hot", articleHandler.UserGetHotArticle)
		userGroup.GET("/article/detail", articleHandler.UserGetArticleDetail)
		userGroup.GET("/article/timeline", articleHandler.UserGetTimeline)

		// 标签相关
		userGroup.GET("/tag/list", tagHandler.UserGetTagList)
		userGroup.GET("/tag/article-list", tagHandler.UserGetArticleListByTag)

		// 友链相关
		userGroup.GET("/link", linkHandler.UserGetLinkList)

		// 管理员相关
		userGroup.GET("/about-me", adminHandler.UserGetAboutMe)
	}

	return r
}

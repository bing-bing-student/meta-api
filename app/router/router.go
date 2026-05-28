package router

import (
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"go.uber.org/dig"
	"go.uber.org/zap"

	"meta-api/app/handler/admin"
	"meta-api/app/handler/article"
	"meta-api/app/handler/link"
	"meta-api/app/handler/tag"
	"meta-api/bootstrap"
	"meta-api/common/middlewares"
	"meta-api/common/utils"
)

// SetUpRouter 启动路由
// container 由调用方（app 层）统一构建并传入，避免重复创建容器导致依赖实例发散
func SetUpRouter(bs *bootstrap.Bootstrap, container *dig.Container) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	logger := bs.Logger

	// 信任 Nginx 代理所在网段的请求
	if err := r.SetTrustedProxies([]string{"172.16.0.0/12"}); err != nil {
		logger.Error("error set trusted proxy", zap.Error(err))
		return nil
	}

	// 添加中间件
	r.Use(middlewares.TimeoutMiddleware(3*time.Second), middlewares.GinLogger(logger), middlewares.GinRecovery(logger, true))

	// 获取 adminHandler 实例
	var adminHandler admin.Handler
	if err := container.Invoke(func(h admin.Handler) { adminHandler = h }); err != nil {
		logger.Error("failed to get admin handler", zap.Error(err))
		return nil
	}

	// 获取 articleHandler 实例
	var articleHandler article.Handler
	if err := container.Invoke(func(h article.Handler) { articleHandler = h }); err != nil {
		logger.Error("failed to get article handler", zap.Error(err))
		return nil
	}

	// 获取 tagHandler 实例
	var tagHandler tag.Handler
	if err := container.Invoke(func(h tag.Handler) { tagHandler = h }); err != nil {
		logger.Error("failed to get tag handler", zap.Error(err))
		return nil
	}

	// 获取 linkHandler 实例
	var linkHandler link.Handler
	if err := container.Invoke(func(h link.Handler) { linkHandler = h }); err != nil {
		logger.Error("failed to get link handler", zap.Error(err))
		return nil
	}

	// 后台管理路由(不需要JWT验证)
	adminGroup := r.Group("/admin")
	{
		adminGroup.POST("/refresh-token", adminHandler.RefreshToken)            // 刷新RefreshToken
		adminGroup.POST("/sms-code", adminHandler.SendSMSCode)                  // 发送短信验证码
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
	r.POST("fingerprint/decrypt", adminHandler.FingerprintDecrypt)
	authorizationKey, _ := utils.GenerateRandomBytes(32)
	encryptionKey, _ := utils.GenerateRandomBytes(16)
	userGroup := r.Group("/user")
	userGroup.Use(sessions.Sessions("session_id", cookie.NewStore(authorizationKey, encryptionKey)))
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

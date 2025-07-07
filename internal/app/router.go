package app

import (
	"os"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"meta-api/internal/app/handler"
	"meta-api/internal/app/handler/admin"
	"meta-api/internal/app/handler/article"
	"meta-api/internal/app/model"
	"meta-api/internal/app/service"
	"meta-api/internal/bootstrap"
	"meta-api/internal/common/middlewares"
)

// SetUpRouter 启动路由
func SetUpRouter(bs *bootstrap.Bootstrap) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
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

	modelContainer := model.NewModel(bs.MySQL, bs.Redis)
	serviceContainer := service.NewService(bs.Config, logger, bs.IDGenerator, modelContainer)
	handlerContainer := handler.NewHandler(serviceContainer)

	// 后台管理路由(不需要JWT验证)
	adminGroup := r.Group("/admin")
	{
		adminGroup.POST("/refresh-token", handlerContainer.RefreshTokenToLogin) // 刷新RefreshToken
		//adminGroup.POST("/sms-code", container.AdminHandler().SendSMSCode)                  // 发送短信验证码
		//adminGroup.POST("/sms-login", container.AdminHandler().SMSLogin)                    // 短信验证码登录
		//adminGroup.POST("/account-login", container.AdminHandler().AccountLogin)            // 账号密码登录
		//adminGroup.POST("/bind-dynamic-code", container.AdminHandler().BindDynamicCode)     // 绑定TOTP动态码
		//adminGroup.POST("/verify-dynamic-code", container.AdminHandler().VerifyDynamicCode) // 验证TOTP动态码
	}

	// 后台管理路由(需要JWT验证)
	authAdminGroup := adminGroup.Group("/auth")
	//authAdminGroup.Use(middlewares.JWT())
	authAdminGroup.Use()
	{
		// 文章管理
		authAdminGroup.GET("/article/list", article.AdminGetArticleList)
		//authAdminGroup.GET("/article/detail", container.ArticleHandler().AdminGetArticleDetail)
		//authAdminGroup.POST("/article/add", container.ArticleHandler().AddArticle)
		//authAdminGroup.PUT("/article/update", container.ArticleHandler().UpdateArticle)
		//authAdminGroup.DELETE("/article/delete", container.ArticleHandler().DeleteArticle)

		// 标签管理
		//authAdminGroup.GET("/tag/list", container.TagHandler().AdminGetTagList)
		//authAdminGroup.GET("/tag/article-list", container.TagHandler().AdminGetArticleListByTag)
		//authAdminGroup.PUT("/tag/update", container.TagHandler().UpdateTag)

		// 友链管理
		//authAdminGroup.GET("/link/list", container.LinkHandler().AdminGetLinkList)
		//authAdminGroup.POST("/link/add", container.LinkHandler().AddLink)
		//authAdminGroup.PUT("/link/update", container.LinkHandler().UpdateLink)
		//authAdminGroup.DELETE("/link/delete", container.LinkHandler().DeleteLink)

		// 管理员相关
		//authAdminGroup.PUT("/about-me", container.AdminHandler().UpdateAdminAboutMe)
	}

	// 前台展示
	userGroup := r.Group("/user")
	userGroup.Use(sessions.Sessions("session_id", cookie.NewStore([]byte(os.Getenv("AUTHORIZATION_KEY")), []byte(os.Getenv("ENCRYPTION_KEY")))))
	{
		//userGroup.GET("/article/list", article.UserGetArticleList)
		//userGroup.GET("/article/search", article.SearchArticle)
		//userGroup.GET("/article/hot", article.GetHotArticle)
		//userGroup.GET("/article/detail", article.UserGetArticleDetail)
		//
		//userGroup.GET("/tag/list", tag.UserGetTagList)
		//userGroup.GET("/tag/article-list", tag.UserGetArticleListByTag)
		//
		//userGroup.GET("/timeline", article.GetTimeline)
		//userGroup.GET("/link", link.UserGetLinkList)
		//userGroup.GET("/about-me", admin.GetAboutMe)
	}

	return r
}

package router

import (
	"os"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"meta-api/internal/app/middleware"
)

// SetUpRouter 启动路由
func SetUpRouter(logger *zap.Logger) *gin.Engine {
	r := gin.New()

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
	r.Use(middleware.TimeoutMiddleware(2*time.Second), cors.New(corsConfig), middleware.GinLogger(logger), middleware.GinRecovery(logger, true))
	//r.Use(middleware.TimeoutMiddleware(2*time.Second), middleware.GinLogger(logger), middleware.GinRecovery(logger, true))

	// 后台管理
	adminGroup := r.Group("/admin")
	//adminGroup.POST("/sms-code", admin.SMSCode)
	//adminGroup.POST("/sms-login", admin.SMSLogin)
	//adminGroup.POST("/account-login", admin.AccountLogin)
	//adminGroup.POST("/bind-dynamic-code", admin.BindDynamicCode)
	//adminGroup.POST("/verify-dynamic-code", admin.VerifyDynamicCode)
	//adminGroup.POST("/refresh-token", token.RefreshToken)
	adminGroup.Use(middleware.JWT())
	{
		//adminGroup.GET("/article/list", article.AdminGetArticleList)
		//adminGroup.GET("/article/detail", article.AdminGetArticleDetail)
		//adminGroup.POST("/article/add", article.AddArticle)
		//adminGroup.PUT("/article/update", article.UpdateArticle)
		//adminGroup.DELETE("/article/delete", article.DeleteArticle)
		//
		//adminGroup.GET("/tag/list", tag.AdminGetTagList)
		//adminGroup.GET("/tag/article-list", tag.AdminGetArticleListByTag)
		//adminGroup.PUT("/tag/update", tag.UpdateTag)
		//
		//adminGroup.GET("/link/list", link.AdminGetLinkList)
		//adminGroup.POST("/link/add", link.AddLink)
		//adminGroup.PUT("/link/update", link.UpdateLink)
		//adminGroup.DELETE("/link/delete", link.DeleteLink)
		//
		//adminGroup.PUT("/about-me", admin.UpdateAdminAboutMe)
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

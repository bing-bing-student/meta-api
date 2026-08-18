package router

import (
	"fmt"

	"go.uber.org/dig"

	"meta-api/app/handler/admin"
	"meta-api/app/handler/article"
	"meta-api/app/handler/comment"
	"meta-api/app/handler/jsonshare"
	"meta-api/app/handler/link"
	"meta-api/app/handler/sitedynamic"
	"meta-api/app/handler/tag"
	"meta-api/app/handler/userauth"
	"meta-api/app/handler/viewlog"
)

type routeHandlers struct {
	admin       admin.Handler
	article     article.Handler
	comment     comment.Handler
	jsonShare   jsonshare.Handler
	link        link.Handler
	siteDynamic sitedynamic.Handler
	tag         tag.Handler
	userAuth    userauth.Handler
	viewLog     viewlog.Handler
}

// resolveHandlers 注册路由处理函数
func resolveHandlers(container *dig.Container) (*routeHandlers, error) {
	handlers := &routeHandlers{}
	err := container.Invoke(func(
		adminHandler admin.Handler,
		articleHandler article.Handler,
		commentHandler comment.Handler,
		jsonShareHandler jsonshare.Handler,
		linkHandler link.Handler,
		siteDynamicHandler sitedynamic.Handler,
		tagHandler tag.Handler,
		userAuthHandler userauth.Handler,
		viewLogHandler viewlog.Handler,
	) {
		handlers.admin = adminHandler
		handlers.article = articleHandler
		handlers.comment = commentHandler
		handlers.jsonShare = jsonShareHandler
		handlers.link = linkHandler
		handlers.siteDynamic = siteDynamicHandler
		handlers.tag = tagHandler
		handlers.userAuth = userAuthHandler
		handlers.viewLog = viewLogHandler
	})
	if err != nil {
		return nil, fmt.Errorf("resolve route handlers: %w", err)
	}
	return handlers, nil
}

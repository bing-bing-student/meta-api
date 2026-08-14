package di

import (
	"go.uber.org/dig"

	adminHandler "meta-api/app/handler/admin"
	articleHandler "meta-api/app/handler/article"
	commentHandler "meta-api/app/handler/comment"
	jsonshareHandler "meta-api/app/handler/jsonshare"
	linkHandler "meta-api/app/handler/link"
	tagHandler "meta-api/app/handler/tag"
	userAuthHandler "meta-api/app/handler/userauth"
	viewLogHandler "meta-api/app/handler/viewlog"

	adminModel "meta-api/app/model/admin"
	articleModel "meta-api/app/model/article"
	commentModel "meta-api/app/model/comment"
	linkModel "meta-api/app/model/link"
	tagModel "meta-api/app/model/tag"
	userModel "meta-api/app/model/user"

	adminService "meta-api/app/service/admin"
	articleService "meta-api/app/service/article"
	commentService "meta-api/app/service/comment"
	jsonshareService "meta-api/app/service/jsonshare"
	linkService "meta-api/app/service/link"
	tagService "meta-api/app/service/tag"
	userAuthService "meta-api/app/service/userauth"
	viewLogService "meta-api/app/service/viewlog"
)

func registerModelProviders(container *dig.Container) error {
	providers := []provider{
		{name: "admin model", constructor: adminModel.NewModel},
		{name: "article model", constructor: articleModel.NewModel},
		{name: "comment model", constructor: commentModel.NewModel},
		{name: "link model", constructor: linkModel.NewModel},
		{name: "tag model", constructor: tagModel.NewModel},
		{name: "user model", constructor: userModel.NewModel},
	}
	return provideAll(container, providers)
}

func registerServiceProviders(container *dig.Container) error {
	providers := []provider{
		{name: "admin service", constructor: adminService.NewService},
		{name: "article service", constructor: articleService.NewService},
		{name: "comment service", constructor: commentService.NewService},
		{name: "jsonshare service", constructor: jsonshareService.NewService},
		{name: "link service", constructor: linkService.NewService},
		{name: "tag service", constructor: tagService.NewService},
		{name: "user auth service", constructor: userAuthService.NewService},
		{name: "view log service", constructor: viewLogService.NewService},
	}
	return provideAll(container, providers)
}

func registerHandlerProviders(container *dig.Container) error {
	providers := []provider{
		{name: "admin handler", constructor: adminHandler.NewHandler},
		{name: "article handler", constructor: articleHandler.NewHandler},
		{name: "comment handler", constructor: commentHandler.NewHandler},
		{name: "jsonshare handler", constructor: jsonshareHandler.NewHandler},
		{name: "link handler", constructor: linkHandler.NewHandler},
		{name: "tag handler", constructor: tagHandler.NewHandler},
		{name: "user auth handler", constructor: userAuthHandler.NewHandler},
		{name: "view log handler", constructor: viewLogHandler.NewHandler},
	}
	return provideAll(container, providers)
}

func provideAll(container *dig.Container, providers []provider) error {
	for _, provider := range providers {
		if err := provide(container, provider.name, provider.constructor); err != nil {
			return err
		}
	}
	return nil
}

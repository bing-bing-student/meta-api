package main

import (
	"context"
	"fmt"
	"log"

	"go.uber.org/dig"

	"meta-api/app/di"
	articleService "meta-api/app/service/article"
	"meta-api/bootstrap"
)

func main() {
	runtimeEnv, err := bootstrap.LoadStartupRuntimeEnv()
	if err != nil {
		log.Fatalf("validate environment failed: %v", err)
	}

	bs := bootstrap.New(runtimeEnv)
	bs.InitConfig()
	bs.InitLogger()
	bs.InitIDGenerator()
	bs.InitMySQL()
	bs.InitRedis()
	bs.InitKeyManager()
	defer bs.Stop()

	container, err := di.BuildContainer(bs)
	if err != nil {
		log.Fatalf("build container failed: %v", err)
	}

	if err = runScan(context.Background(), container); err != nil {
		log.Fatalf("scan article images failed: %v", err)
	}
}

func runScan(ctx context.Context, container *dig.Container) error {
	return container.Invoke(func(service articleService.Service) error {
		result, err := service.AdminScanArticleImages(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("article_total=%d\n", result.ArticleTotal)
		fmt.Printf("image_total=%d\n", result.ImageTotal)
		fmt.Printf("reference_total=%d\n", result.ReferenceTotal)
		fmt.Printf("used_total=%d\n", result.UsedTotal)
		fmt.Printf("unused_total=%d\n", result.UnusedTotal)
		fmt.Printf("missing_total=%d\n", result.MissingTotal)
		return nil
	})
}

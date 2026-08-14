package di

import (
	"fmt"

	"go.uber.org/dig"

	"meta-api/bootstrap"
)

type provider struct {
	name        string
	constructor any
}

type registrar struct {
	name string
	fn   func(*dig.Container) error
}

// BuildContainer 依赖注入容器
func BuildContainer(bs *bootstrap.Bootstrap) (*dig.Container, error) {
	if err := validateBootstrap(bs); err != nil {
		return nil, err
	}

	container := dig.New()

	registrars := []registrar{
		{name: "base", fn: func(c *dig.Container) error { return registerBaseProviders(c, bs) }},
		{name: "model", fn: registerModelProviders},
		{name: "service", fn: registerServiceProviders},
		{name: "handler", fn: registerHandlerProviders},
	}

	for _, registrar := range registrars {
		if err := registrar.fn(container); err != nil {
			return nil, fmt.Errorf("register %s providers: %w", registrar.name, err)
		}
	}

	return container, nil
}

func provide(container *dig.Container, name string, constructor any) error {
	if err := container.Provide(constructor); err != nil {
		return fmt.Errorf("provide %s: %w", name, err)
	}
	return nil
}

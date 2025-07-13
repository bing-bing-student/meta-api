package link

import (
	"go.uber.org/zap"

	"meta-api/app/service/link"
)

type Handler interface {
}

type linkHandler struct {
	logger  *zap.Logger
	service link.Service
}

func NewHandler(logger *zap.Logger, service link.Service) Handler {
	return &linkHandler{
		logger:  logger,
		service: service,
	}
}

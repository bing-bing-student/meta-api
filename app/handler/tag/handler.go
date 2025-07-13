package tag

import (
	"go.uber.org/zap"

	"meta-api/app/service/tag"
)

type Handler interface {
}

type tagHandler struct {
	logger  *zap.Logger
	service tag.Service
}

func NewHandler(logger *zap.Logger, service tag.Service) Handler {
	return &tagHandler{
		logger:  logger,
		service: service,
	}
}

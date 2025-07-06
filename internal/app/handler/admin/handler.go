package admin

import (
	"meta-api/internal/app/service/admin"
)

// Handler 依赖注入
type Handler struct {
	service *admin.Service
}

// NewHandler 创建 Handler
func NewHandler(service *admin.Service) *Handler {
	return &Handler{
		service: service,
	}
}

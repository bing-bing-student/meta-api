package admin

import (
	"context"

	"gorm.io/gorm"
)

type Model interface {
	AddAdminSecretKey(ctx context.Context, id uint64, secretKey string) error
	GetAdminSecretKey(ctx context.Context, id uint64) (string, error)
}

type adminModel struct {
	mysql *gorm.DB
}

func NewModel(mysql *gorm.DB) Model {
	return &adminModel{mysql: mysql}
}

package admin

import (
	"context"

	"gorm.io/gorm"
)

type Model interface {
	AddAdminSecretKey(ctx context.Context, id uint64, secretKey string) error
	GetAdminSecretKey(ctx context.Context, id uint64) (string, error)
	PhoneNumberExist(ctx context.Context, phone string) (string, error)
	CheckAccount(ctx context.Context, username string, password string) (*Admin, error)
	GetAdminInfoByID(ctx context.Context, id uint64) (*AdministerInfo, error)
	GetAdminInfo(ctx context.Context) (*AdministerInfo, error)
	UpdateAdminInfoByID(ctx context.Context, id uint64, info *Admin) error
}

type adminModel struct {
	mysql *gorm.DB
}

func NewModel(mysql *gorm.DB) Model {
	return &adminModel{mysql: mysql}
}

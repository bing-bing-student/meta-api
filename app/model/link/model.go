package link

import (
	"gorm.io/gorm"
)

type Model interface {
}

type linkModel struct {
	mysql *gorm.DB
}

func NewModel(mysql *gorm.DB) Model {
	return &linkModel{mysql: mysql}
}

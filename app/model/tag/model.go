package tag

import (
	"gorm.io/gorm"
)

type Model interface {
}

type tagModel struct {
	mysql *gorm.DB
}

func NewModel(mysql *gorm.DB) Model {
	return &tagModel{mysql: mysql}
}

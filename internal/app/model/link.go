package model

import (
	"time"
)

type Link struct {
	ID         uint64    `gorm:"primary_key;NOT NULL"`
	Name       string    `gorm:"NOT NULL;unique"`
	URL        string    `gorm:"NOT NULL;unique"`
	CreateTime time.Time `gorm:"column:create_time" json:"createTime"`
	UpdateTime time.Time `gorm:"column:update_time" json:"updateTime"`
}

package article

import (
	"time"

	"meta-api/internal/app/model/tag"
)

type Article struct {
	ID         uint64    `gorm:"primary_key;NOT NULL"`
	Title      string    `gorm:"NOT NULL;unique"`
	Describe   string    `gorm:"NOT NULL"`
	Content    string    `gorm:"type:text;NOT NULL"`
	ViewNum    uint64    `gorm:"NOT NULL"`
	CreateTime time.Time `gorm:"NOT NULL"`
	UpdateTime time.Time `gorm:"NOT NULL"`
	TagID      uint64    `gorm:"NOT NULL"`
	Tag        tag.Tag   `gorm:"foreignKey:TagID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
}

type Detail struct {
	ID         uint64    `gorm:"column:id" json:"id"`
	Title      string    `gorm:"column:title" json:"title"`
	Describe   string    `gorm:"column:describe" json:"describe"`
	Content    string    `gorm:"column:content" json:"content"`
	ViewNum    uint64    `gorm:"column:view_num" json:"viewNum"`
	CreateTime time.Time `gorm:"column:create_time" json:"createTime"`
	UpdateTime time.Time `gorm:"column:update_time" json:"updateTime"`
	TagID      uint64    `gorm:"column:tag_id" json:"tagID"`
	TagName    string    `gorm:"column:tag_name" json:"tagName"`
}

type DelArticle struct {
	ID      uint64 `gorm:"column:id" json:"id"`
	TagID   uint64 `gorm:"column:tag_id" json:"tagID"`
	TagName string `gorm:"column:tag_name" json:"tagName"`
}

type TagWithArticleCount struct {
	Name  string `gorm:"column:name"`
	Count int    `gorm:"column:count"`
}

type TagNameArticleZSet struct {
	ID         uint64    `gorm:"column:id" json:"ID"`
	CreateTime time.Time `gorm:"column:create_time" json:"createTime"`
}

type TimeAndViewZSet struct {
	ID         uint64    `gorm:"column:id" json:"ID"`
	ViewNum    uint64    `gorm:"column:view_num" json:"viewNum"`
	CreateTime time.Time `gorm:"column:create_time" json:"createTime"`
}

// GetArticleDetailByID 通过文章ID获取文章信息
//func GetArticleDetailByID(id uint64) (detail *Detail, err error) {
//	detail = new(Detail)
//	if err = global.MySqlDB.
//		Table("article as a").
//		Select("a.id, a.title, a.describe, a.content, a.view_num, a.create_time, a.update_time, a.tag_id, b.name as tag_name").
//		Joins("JOIN tag as b ON a.tag_id=b.id").
//		Where("a.id = ?", id).
//		First(detail).Error; err != nil {
//		return nil, err
//	}
//	return detail, nil
//}

// GetTagNameArticleZSetByTagName 通过标签名称获取文章信息的ZSet
//func GetTagNameArticleZSetByTagName(tagName string) (tagNameArticleZSet []TagNameArticleZSet, err error) {
//	if err = global.MySqlDB.Model(&Article{}).
//		Joins("JOIN tag ON tag.id = article.tag_id").
//		Where("tag.name = ?", tagName).
//		Select("article.id, article.create_time").
//		Find(&tagNameArticleZSet).Error; err != nil {
//		return nil, err
//	}
//
//	return tagNameArticleZSet, nil
//}

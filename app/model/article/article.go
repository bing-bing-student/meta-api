package article

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"meta-api/app/model/tag"
	"meta-api/common/constants"
	"meta-api/common/utils"
)

type Article struct {
	ID            uint64     `gorm:"primary_key;NOT NULL"`
	Title         string     `gorm:"type:varchar(100);NOT NULL;default:''"`
	Describe      string     `gorm:"type:varchar(200);NOT NULL;default:''"`
	Content       string     `gorm:"type:mediumtext;NOT NULL"`
	ViewNum       uint64     `gorm:"NOT NULL;default:0"`
	Status        string     `gorm:"column:status;type:varchar(20);NOT NULL;default:published;index"`
	PublishedID   *uint64    `gorm:"column:published_id;uniqueIndex"`
	PublishedTime *time.Time `gorm:"column:published_time"`
	CreateTime    time.Time  `gorm:"NOT NULL"`
	UpdateTime    time.Time  `gorm:"NOT NULL"`
	TagID         *uint64    `gorm:"column:tag_id;index"`
	Tag           tag.Tag    `gorm:"foreignKey:TagID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
}

type Detail struct {
	ID            uint64     `gorm:"column:id" json:"id"`
	Title         string     `gorm:"column:title" json:"title"`
	Describe      string     `gorm:"column:describe" json:"describe"`
	Content       string     `gorm:"column:content" json:"content"`
	ViewNum       uint64     `gorm:"column:view_num" json:"viewNum"`
	Status        string     `gorm:"column:status" json:"status"`
	PublishedID   *uint64    `gorm:"column:published_id" json:"publishedID"`
	PublishedTime *time.Time `gorm:"column:published_time" json:"publishedTime"`
	CreateTime    time.Time  `gorm:"column:create_time" json:"createTime"`
	UpdateTime    time.Time  `gorm:"column:update_time" json:"updateTime"`
	TagID         *uint64    `gorm:"column:tag_id" json:"tagID"`
	TagName       string     `gorm:"column:tag_name" json:"tagName"`
}

type SearchArticle struct {
	ID         uint64    `gorm:"column:id" json:"id"`
	Title      string    `gorm:"column:title" json:"title"`
	Describe   string    `gorm:"column:describe" json:"describe"`
	ViewNum    uint64    `gorm:"column:view_num" json:"viewNum"`
	CreateTime time.Time `gorm:"column:create_time" json:"createTime"`
}

type DelArticle struct {
	ID      uint64 `gorm:"column:id" json:"id"`
	TagID   uint64 `gorm:"column:tag_id" json:"tagID"`
	TagName string `gorm:"column:tag_name" json:"tagName"`
}

type ListByTagName struct {
	ID         uint64    `gorm:"column:id" json:"ID"`
	CreateTime time.Time `gorm:"column:create_time" json:"createTime"`
}

type TimeAndViewZSet struct {
	ID         uint64    `gorm:"column:id" json:"ID"`
	ViewNum    uint64    `gorm:"column:view_num" json:"viewNum"`
	CreateTime time.Time `gorm:"column:create_time" json:"createTime"`
}

const (
	ArticleStatusDraft     = constants.ArticleStatusDraft
	ArticleStatusPublished = constants.ArticleStatusPublished
)

// ViewNumUpdate 文章浏览量批量更新项
type ViewNumUpdate struct {
	ID      string
	ViewNum int
}

// CreateArticle 创建文章
func (a *articleModel) CreateArticle(ctx context.Context, newArticle *Article) error {
	if newArticle.Status == "" {
		newArticle.Status = ArticleStatusPublished
	}
	if err := a.mysql.WithContext(ctx).Model(&Article{}).Create(newArticle).Error; err != nil {
		return fmt.Errorf("failed to create article: %w", err)
	}

	return nil
}

// UpdateArticle 更新文章
func (a *articleModel) UpdateArticle(ctx context.Context, articleInfo *Article) error {
	if err := a.mysql.WithContext(ctx).Model(&Article{}).
		Where("id = ? AND status = ?", articleInfo.ID, ArticleStatusPublished).Updates(articleInfo).Error; err != nil {
		return fmt.Errorf("failed to update article: %w", err)
	}

	return nil
}

// UpdateArticleViewNum 更新文章浏览量
func (a *articleModel) UpdateArticleViewNum(ctx context.Context, id string, viewNum float64) error {
	if err := a.mysql.WithContext(ctx).Model(&Article{}).
		Where("id = ? AND status = ?", id, ArticleStatusPublished).Update("view_num", viewNum).Error; err != nil {
		return fmt.Errorf("failed to update article: %w", err)
	}

	return nil
}

// GetArticleDetailByID 通过文章ID获取文章详情
func (a *articleModel) GetArticleDetailByID(ctx context.Context, id uint64) (*Detail, error) {
	detail := &Detail{}
	if err := a.mysql.WithContext(ctx).Model(&Article{}).
		Table("article as a").
		Select("a.id, a.title, a.describe, a.content, a.view_num, a.status, a.published_id, a.published_time, a.create_time, a.update_time, a.tag_id, COALESCE(b.name, '') as tag_name").
		Joins("LEFT JOIN tag as b ON a.tag_id=b.id").
		Where("a.id = ? AND a.status = ?", id, ArticleStatusPublished).
		First(detail).Error; err != nil {
		return nil, err
	}

	return detail, nil
}

// GetArticleListByTagName 通过标签名称获取文章信息
func (a *articleModel) GetArticleListByTagName(ctx context.Context, tagName string) ([]ListByTagName, error) {
	list := make([]ListByTagName, 0)
	if err := a.mysql.WithContext(ctx).Model(&Article{}).
		Select("article.id, article.create_time").
		Joins("JOIN tag ON tag.id = article.tag_id").
		Where("tag.name = ? AND article.status = ?", tagName, ArticleStatusPublished).
		Find(&list).Error; err != nil {
		return nil, err
	}

	return list, nil
}

// GetArticleDeleteInfo 获取删除文章需要的元信息。
func (a *articleModel) GetArticleDeleteInfo(ctx context.Context, id uint64) (string, error) {
	articleInfo := &DelArticle{}
	if err := a.mysql.WithContext(ctx).Model(&Article{}).Table("article as a").
		Select("a.id, a.tag_id, t.name as tag_name").
		Joins("LEFT JOIN tag as t ON a.tag_id = t.id").
		Where("a.id = ? AND a.status = ?", id, ArticleStatusPublished).
		First(articleInfo).Error; err != nil {
		return "", err
	}

	if articleInfo.ID != 0 && articleInfo.TagName != "" {
		return articleInfo.TagName, nil
	}

	return "", fmt.Errorf("article not exist")
}

// DeleteArticleByID 硬删除文章。
func (a *articleModel) DeleteArticleByID(ctx context.Context, id uint64) error {
	return a.mysql.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("published_id = ? AND status = ?", id, ArticleStatusDraft).
			Delete(&Article{}).Error; err != nil {
			return fmt.Errorf("failed to delete article drafts: %w", err)
		}
		if err := tx.Where("id = ? AND status = ?", id, ArticleStatusPublished).
			Delete(&Article{}).Error; err != nil {
			return fmt.Errorf("failed to delete article: %w", err)
		}
		return nil
	})
}

// SearchArticle 搜索文章
//
// 标题与正文均按子串匹配（LIKE）：标题是短的高信号字段、正文是长字段，
// 文章量级很小（线上 ≤500 篇），全表扫描即为毫秒级，无需全文索引。
// 排序上让标题命中的优先，其次按浏览量近似相关度。
func (a *articleModel) SearchArticle(ctx context.Context, word string, limit, offset int) ([]SearchArticle, int64, error) {
	list := make([]SearchArticle, 0)
	total := int64(0)
	like := "%" + utils.EscapeLike(word) + "%"

	// 第一次查询：统计命中总数（不分页），用于前端分页
	if err := a.mysql.WithContext(ctx).Model(&Article{}).
		Where("status = ? AND (title LIKE ? COLLATE utf8mb4_general_ci OR content LIKE ?)",
			ArticleStatusPublished, like, like).
		Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count articles: %w", err)
	}
	if total == 0 {
		return list, 0, nil
	}

	// 第二次查询：取当前分页的数据，标题命中优先、再按浏览量排序
	if err := a.mysql.WithContext(ctx).Model(&Article{}).
		Select("`id`, `title`, `describe`, `view_num`, `create_time`, (`title` LIKE ? COLLATE utf8mb4_general_ci) AS title_hit", like).
		Where("status = ? AND (title LIKE ? COLLATE utf8mb4_general_ci OR content LIKE ?)",
			ArticleStatusPublished, like, like).
		Order("title_hit DESC, view_num DESC").
		Limit(limit).Offset(offset).
		Find(&list).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to query articles: %w", err)
	}
	return list, total, nil
}

// GetArticleListByIDList 通过id列表获取文章列表
func (a *articleModel) GetArticleListByIDList(ctx context.Context, ids []uint64) ([]*Article, error) {
	var articles []*Article
	err := a.mysql.WithContext(ctx).Model(&Article{}).
		Preload("Tag").
		Where("id IN ? AND status = ?", ids, ArticleStatusPublished).
		Find(&articles).Error
	return articles, err
}

// UpdateArticleTagID 更新文章的tagID
func (a *articleModel) UpdateArticleTagID(ctx context.Context, articleIDList []string, tagID uint64) error {
	if err := a.mysql.WithContext(ctx).Model(&Article{}).
		Where("id IN ? AND status = ?", articleIDList, ArticleStatusPublished).
		Update("tag_id", tagID).Error; err != nil {
		return err
	}
	return nil
}

// GetArticleList 获取文章列表（带分页）
func (a *articleModel) GetArticleList(ctx context.Context, offset, limit int) ([]*Article, error) {
	var articles []*Article

	// 使用 Preload 加载关联的标签数据
	// 按创建时间倒序排列（根据需求可调整排序字段）
	err := a.mysql.WithContext(ctx).
		Preload("Tag").
		Where("status = ?", ArticleStatusPublished).
		Order("create_time DESC").
		Offset(offset).
		Limit(limit).
		Find(&articles).Error

	if err != nil {
		return nil, err
	}
	return articles, nil
}

// GetArticleCount 获取文章总数
func (a *articleModel) GetArticleCount(ctx context.Context) (int, error) {
	var count int64
	err := a.mysql.WithContext(ctx).
		Model(&Article{}).
		Where("status = ?", ArticleStatusPublished).
		Count(&count).Error
	return int(count), err
}

// ListTimeAndView 拉取所有文章的 ID/浏览量/创建时间，用于缓存预热
func (a *articleModel) ListTimeAndView(ctx context.Context) ([]TimeAndViewZSet, error) {
	list := make([]TimeAndViewZSet, 0)
	if err := a.mysql.WithContext(ctx).
		Model(&Article{}).
		Select("id", "view_num", "create_time").
		Where("status = ?", ArticleStatusPublished).
		Find(&list).Error; err != nil {
		return nil, fmt.Errorf("failed to list time and view: %w", err)
	}
	return list, nil
}

// BatchUpdateViewNum 批量回写浏览量到数据库
// 使用 CASE WHEN 单条 SQL 完成 N 行更新，显著降低 RTT 与持久化耗时
func (a *articleModel) BatchUpdateViewNum(ctx context.Context, items []ViewNumUpdate) error {
	if len(items) == 0 {
		return nil
	}

	var sb strings.Builder
	sb.WriteString("UPDATE article SET view_num = CASE id ")

	args := make([]any, 0, len(items)*2+len(items))
	ids := make([]any, 0, len(items))
	for _, it := range items {
		sb.WriteString("WHEN ? THEN ? ")
		args = append(args, it.ID, it.ViewNum)
		ids = append(ids, it.ID)
	}
	sb.WriteString("END WHERE id IN ? AND status = ?")
	args = append(args, ids)
	args = append(args, ArticleStatusPublished)

	if err := a.mysql.WithContext(ctx).Exec(sb.String(), args...).Error; err != nil {
		return fmt.Errorf("failed to batch update view num: %w", err)
	}
	return nil
}

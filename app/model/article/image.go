package article

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	ImageStatusUsed    = "used"
	ImageStatusUnused  = "unused"
	ImageStatusMissing = "missing"

	ImageSourceUpload = "upload"
	ImageSourceScan   = "scan"
	ImageSourceCOS    = "cos"

	ImageRefTypeMarkdown = "markdown"
	ImageRefTypeHTML     = "html"
)

type ArticleImage struct {
	ID                 uint64     `gorm:"primary_key;NOT NULL"`
	ObjectKey          string     `gorm:"column:object_key;type:varchar(500);NOT NULL;uniqueIndex"`
	URL                string     `gorm:"column:url;type:varchar(1000);NOT NULL"`
	ImageName          string     `gorm:"column:image_name;type:varchar(255);NOT NULL"`
	Mime               string     `gorm:"column:mime;type:varchar(100);NOT NULL;default:''"`
	Size               int64      `gorm:"column:size;NOT NULL;default:0"`
	ETag               string     `gorm:"column:etag;type:varchar(128);NOT NULL;default:''"`
	Status             string     `gorm:"column:status;type:varchar(20);NOT NULL;index"`
	Source             string     `gorm:"column:source;type:varchar(20);NOT NULL;index"`
	LastSeenTime       *time.Time `gorm:"column:last_seen_time;index"`
	ObjectModifiedTime *time.Time `gorm:"column:object_modified_time"`
	CreateTime         time.Time  `gorm:"column:create_time;NOT NULL"`
	UpdateTime         time.Time  `gorm:"column:update_time;NOT NULL"`
}

type ArticleImageReference struct {
	ID         uint64 `gorm:"primary_key;NOT NULL"`
	ImageID    uint64 `gorm:"column:image_id;NOT NULL;uniqueIndex:idx_article_image_ref,priority:1;index"`
	ArticleID  uint64 `gorm:"column:article_id;NOT NULL;uniqueIndex:idx_article_image_ref,priority:2;index"`
	RefType    string `gorm:"column:ref_type;type:varchar(20);NOT NULL;uniqueIndex:idx_article_image_ref,priority:3"`
	RefCount   int    `gorm:"column:ref_count;NOT NULL;default:0"`
	CreateTime time.Time
	UpdateTime time.Time
	Image      ArticleImage `gorm:"foreignKey:ImageID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Article    Article      `gorm:"foreignKey:ArticleID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
}

type ArticleImageSource struct {
	ID      uint64 `gorm:"column:id"`
	Title   string `gorm:"column:title"`
	Content string `gorm:"column:content"`
}

type ArticleImageQuery struct {
	Status  string
	Keyword string
	Offset  int
	Limit   int
}

type ArticleImageListRecord struct {
	ID                 uint64     `gorm:"column:id"`
	ObjectKey          string     `gorm:"column:object_key"`
	URL                string     `gorm:"column:url"`
	ImageName          string     `gorm:"column:image_name"`
	Mime               string     `gorm:"column:mime"`
	Size               int64      `gorm:"column:size"`
	ETag               string     `gorm:"column:etag"`
	Status             string     `gorm:"column:status"`
	Source             string     `gorm:"column:source"`
	RefCount           int        `gorm:"column:ref_count"`
	LastSeenTime       *time.Time `gorm:"column:last_seen_time"`
	ObjectModifiedTime *time.Time `gorm:"column:object_modified_time"`
	CreateTime         time.Time  `gorm:"column:create_time"`
	UpdateTime         time.Time  `gorm:"column:update_time"`
}

type ArticleImageReferenceRecord struct {
	ID           uint64    `gorm:"column:id"`
	ImageID      uint64    `gorm:"column:image_id"`
	ArticleID    uint64    `gorm:"column:article_id"`
	ArticleTitle string    `gorm:"column:article_title"`
	RefType      string    `gorm:"column:ref_type"`
	RefCount     int       `gorm:"column:ref_count"`
	CreateTime   time.Time `gorm:"column:create_time"`
	UpdateTime   time.Time `gorm:"column:update_time"`
}

func (a *articleModel) ListArticleImageSources(ctx context.Context) ([]ArticleImageSource, error) {
	sources := make([]ArticleImageSource, 0)
	if err := a.mysql.WithContext(ctx).Model(&Article{}).
		Select("id, title, content").
		Where("status = ?", ArticleStatusPublished).
		Find(&sources).Error; err != nil {
		return nil, fmt.Errorf("list article image sources: %w", err)
	}
	return sources, nil
}

func (a *articleModel) FindArticleImagesByObjectKeys(ctx context.Context, objectKeys []string) (map[string]ArticleImage, error) {
	result := make(map[string]ArticleImage, len(objectKeys))
	if len(objectKeys) == 0 {
		return result, nil
	}

	images := make([]ArticleImage, 0)
	if err := a.mysql.WithContext(ctx).Model(&ArticleImage{}).
		Where("object_key IN ?", objectKeys).
		Find(&images).Error; err != nil {
		return nil, fmt.Errorf("find article images by object keys: %w", err)
	}
	for _, image := range images {
		result[image.ObjectKey] = image
	}
	return result, nil
}

func (a *articleModel) SyncArticleImages(ctx context.Context, images []ArticleImage,
	references []ArticleImageReference) error {
	return a.mysql.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if len(images) > 0 {
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "object_key"}},
				DoUpdates: clause.AssignmentColumns([]string{
					"url",
					"image_name",
					"mime",
					"size",
					"etag",
					"status",
					"source",
					"last_seen_time",
					"object_modified_time",
					"update_time",
				}),
			}).CreateInBatches(images, 100).Error; err != nil {
				return fmt.Errorf("upsert article images: %w", err)
			}
		}

		if err := tx.Where("1 = 1").Delete(&ArticleImageReference{}).Error; err != nil {
			return fmt.Errorf("clear article image references: %w", err)
		}
		if len(references) > 0 {
			if err := tx.CreateInBatches(references, 100).Error; err != nil {
				return fmt.Errorf("create article image references: %w", err)
			}
		}
		return nil
	})
}

func (a *articleModel) CreateArticleImage(ctx context.Context, image *ArticleImage) error {
	if err := a.mysql.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "object_key"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"url",
			"image_name",
			"mime",
			"size",
			"etag",
			"status",
			"source",
			"last_seen_time",
			"object_modified_time",
			"update_time",
		}),
	}).Create(image).Error; err != nil {
		return fmt.Errorf("create article image: %w", err)
	}
	return nil
}

func (a *articleModel) ListArticleImages(ctx context.Context,
	query ArticleImageQuery) ([]ArticleImageListRecord, int64, error) {
	applyFilter := func(db *gorm.DB) *gorm.DB {
		if query.Status != "" {
			db = db.Where("status = ?", query.Status)
		}
		if query.Keyword != "" {
			like := "%" + query.Keyword + "%"
			db = db.Where("image_name LIKE ? OR url LIKE ?", like, like)
		}
		return db
	}

	var total int64
	if err := applyFilter(a.mysql.WithContext(ctx).Model(&ArticleImage{})).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count article images: %w", err)
	}
	if total == 0 {
		return []ArticleImageListRecord{}, 0, nil
	}

	records := make([]ArticleImageListRecord, 0)
	refCountSubQuery := a.mysql.Model(&ArticleImageReference{}).
		Select("image_id, SUM(ref_count) AS ref_count").
		Group("image_id")
	if err := applyFilter(a.mysql.WithContext(ctx).Model(&ArticleImage{})).
		Select("article_image.*, COALESCE(refs.ref_count, 0) AS ref_count").
		Joins("LEFT JOIN (?) AS refs ON refs.image_id = article_image.id", refCountSubQuery).
		Order("article_image.update_time DESC, article_image.id DESC").
		Offset(query.Offset).
		Limit(query.Limit).
		Find(&records).Error; err != nil {
		return nil, 0, fmt.Errorf("list article images: %w", err)
	}
	return records, total, nil
}

func (a *articleModel) GetArticleImageByID(ctx context.Context, id uint64) (*ArticleImage, error) {
	image := &ArticleImage{}
	if err := a.mysql.WithContext(ctx).Model(&ArticleImage{}).Where("id = ?", id).First(image).Error; err != nil {
		return nil, err
	}
	return image, nil
}

func (a *articleModel) ListArticleImageReferences(ctx context.Context,
	imageID uint64) ([]ArticleImageReferenceRecord, error) {
	records := make([]ArticleImageReferenceRecord, 0)
	if err := a.mysql.WithContext(ctx).Model(&ArticleImageReference{}).
		Table("article_image_reference AS r").
		Select("r.id, r.image_id, r.article_id, a.title AS article_title, r.ref_type, r.ref_count, r.create_time, r.update_time").
		Joins("LEFT JOIN article AS a ON a.id = r.article_id").
		Where("r.image_id = ?", imageID).
		Order("r.update_time DESC, r.id DESC").
		Find(&records).Error; err != nil {
		return nil, fmt.Errorf("list article image references: %w", err)
	}
	return records, nil
}

func (a *articleModel) CountArticleImageReferences(ctx context.Context, imageID uint64) (int64, error) {
	var count int64
	if err := a.mysql.WithContext(ctx).Model(&ArticleImageReference{}).
		Where("image_id = ?", imageID).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count article image references: %w", err)
	}
	return count, nil
}

func (a *articleModel) DeleteArticleImage(ctx context.Context, id uint64) error {
	return a.mysql.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("image_id = ?", id).Delete(&ArticleImageReference{}).Error; err != nil {
			return fmt.Errorf("delete article image references: %w", err)
		}
		if err := tx.Delete(&ArticleImage{}, id).Error; err != nil {
			return fmt.Errorf("delete article image: %w", err)
		}
		return nil
	})
}

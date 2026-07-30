package bootstrap

import (
	"fmt"

	"gorm.io/gorm"

	adminModel "meta-api/app/model/admin"
	articleModel "meta-api/app/model/article"
	commentModel "meta-api/app/model/comment"
	linkModel "meta-api/app/model/link"
	tagModel "meta-api/app/model/tag"
	userModel "meta-api/app/model/user"
)

// autoMigrateMySQL keeps database tables aligned with GORM model definitions.
// It is intentionally limited to schema creation/additive changes handled by GORM AutoMigrate.
func autoMigrateMySQL(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("mysql db is nil")
	}
	if err := db.AutoMigrate(
		&adminModel.Admin{},
		&tagModel.Tag{},
		&articleModel.Article{},
		&linkModel.Link{},
		&userModel.User{},
		&commentModel.Comment{},
		&commentModel.CommentReport{},
	); err != nil {
		return fmt.Errorf("auto migrate mysql tables: %w", err)
	}
	return nil
}

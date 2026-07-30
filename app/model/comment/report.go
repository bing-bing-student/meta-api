package comment

import (
	"context"
	"fmt"
	"strings"
	"time"

	"meta-api/common/utils"

	"gorm.io/gorm"
)

const (
	ReportStatusPending  = "pending"
	ReportStatusAccepted = "accepted"
	ReportStatusRejected = "rejected"
)

func IsValidReportStatus(status string) bool {
	switch status {
	case ReportStatusPending, ReportStatusAccepted, ReportStatusRejected:
		return true
	default:
		return false
	}
}

type CommentReport struct {
	ID         uint64    `gorm:"primary_key;NOT NULL"`
	CommentID  uint64    `gorm:"column:comment_id;NOT NULL;uniqueIndex:idx_comment_report_comment_reporter,priority:1;index"`
	ArticleID  uint64    `gorm:"column:article_id;NOT NULL;index"`
	ReporterID uint64    `gorm:"column:reporter_id;NOT NULL;uniqueIndex:idx_comment_report_comment_reporter,priority:2;index"`
	Reason     string    `gorm:"type:varchar(200);NOT NULL;default:''"`
	Status     string    `gorm:"type:varchar(20);NOT NULL;default:pending;index"`
	IP         string    `gorm:"type:varchar(64)"`
	CreateTime time.Time `gorm:"column:create_time;NOT NULL;index"`
	UpdateTime time.Time `gorm:"column:update_time;NOT NULL"`
}

type AdminReportListFilter struct {
	CommentQuery   string
	AuthorHandle   string
	ReporterHandle string
	Status         string
	Offset         int
	Limit          int
}

type AdminReportListItem struct {
	ID                  uint64    `gorm:"column:id"`
	CommentID           uint64    `gorm:"column:comment_id"`
	ArticleID           uint64    `gorm:"column:article_id"`
	ArticleTitle        string    `gorm:"column:article_title"`
	CommentAuthorID     uint64    `gorm:"column:comment_author_id"`
	CommentAuthorName   string    `gorm:"column:comment_author_name"`
	CommentAuthorHandle string    `gorm:"column:comment_author_handle"`
	CommentContent      string    `gorm:"column:comment_content"`
	CommentStatus       string    `gorm:"column:comment_status"`
	ReporterID          uint64    `gorm:"column:reporter_id"`
	ReporterName        string    `gorm:"column:reporter_name"`
	ReporterHandle      string    `gorm:"column:reporter_handle"`
	Reason              string    `gorm:"column:reason"`
	Status              string    `gorm:"column:status"`
	IP                  string    `gorm:"column:ip"`
	CreateTime          time.Time `gorm:"column:create_time"`
	UpdateTime          time.Time `gorm:"column:update_time"`
}

func (m *commentModel) CreateCommentReport(ctx context.Context, report *CommentReport,
	threshold int64, updateTime time.Time) (int64, bool, error) {

	if threshold <= 0 {
		threshold = 3
	}

	var pendingCount int64
	movedToPending := false
	err := m.mysql.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&CommentReport{}).Create(report).Error; err != nil {
			return fmt.Errorf("failed to create comment report: %w", err)
		}

		if err := tx.Model(&CommentReport{}).
			Where("comment_id = ? AND status = ?", report.CommentID, ReportStatusPending).
			Count(&pendingCount).Error; err != nil {
			return fmt.Errorf("failed to count comment reports: %w", err)
		}

		if pendingCount < threshold {
			return nil
		}
		result := tx.Model(&Comment{}).
			Where("id = ? AND status = ?", report.CommentID, StatusApproved).
			Updates(map[string]any{
				"status":      StatusPending,
				"update_time": updateTime,
			})
		if result.Error != nil {
			return fmt.Errorf("failed to update reported comment status: %w", result.Error)
		}
		movedToPending = result.RowsAffected > 0
		return nil
	})
	if err != nil {
		return 0, false, err
	}
	return pendingCount, movedToPending, nil
}

func (m *commentModel) GetCommentReportByCommentAndReporter(ctx context.Context,
	commentID uint64, reporterID uint64) (*CommentReport, error) {

	report := &CommentReport{}
	if err := m.mysql.WithContext(ctx).Model(&CommentReport{}).
		Where("comment_id = ? AND reporter_id = ?", commentID, reporterID).
		First(report).Error; err != nil {
		return nil, err
	}
	return report, nil
}

func (m *commentModel) ListReportedCommentIDsByReporter(ctx context.Context,
	commentIDs []uint64, reporterID uint64) ([]uint64, error) {

	if len(commentIDs) == 0 || reporterID == 0 {
		return []uint64{}, nil
	}

	reportedIDs := make([]uint64, 0, len(commentIDs))
	if err := m.mysql.WithContext(ctx).Model(&CommentReport{}).
		Where("reporter_id = ? AND comment_id IN ?", reporterID, commentIDs).
		Pluck("comment_id", &reportedIDs).Error; err != nil {
		if isCommentReportTableMissing(err) {
			return []uint64{}, nil
		}
		return nil, fmt.Errorf("failed to list reported comment ids: %w", err)
	}
	return reportedIDs, nil
}

func (m *commentModel) ListCommentReports(ctx context.Context,
	filter AdminReportListFilter) ([]AdminReportListItem, int64, error) {

	applyFilter := func(db *gorm.DB) *gorm.DB {
		if filter.CommentQuery != "" {
			like := "%" + utils.EscapeLike(filter.CommentQuery) + "%"
			db = db.Where("(CAST(cr.comment_id AS CHAR) LIKE ? OR c.content LIKE ? COLLATE utf8mb4_general_ci)", like, like)
		}
		if filter.AuthorHandle != "" {
			db = db.Where("cu.handle = ?", filter.AuthorHandle)
		}
		if filter.ReporterHandle != "" {
			db = db.Where("ru.handle = ?", filter.ReporterHandle)
		}
		if filter.Status != "" {
			db = db.Where("cr.status = ?", filter.Status)
		}
		return db
	}

	base := func(db *gorm.DB) *gorm.DB {
		return db.Table("comment_report AS cr").
			Joins("LEFT JOIN comment AS c ON c.id = cr.comment_id").
			Joins("LEFT JOIN article AS a ON a.id = cr.article_id").
			Joins("LEFT JOIN `user` AS cu ON cu.id = c.user_id").
			Joins("LEFT JOIN `user` AS ru ON ru.id = cr.reporter_id")
	}

	countQuery := applyFilter(base(m.mysql.WithContext(ctx)))
	var total int64
	if err := countQuery.Count(&total).Error; err != nil {
		if isCommentReportTableMissing(err) {
			return []AdminReportListItem{}, 0, nil
		}
		return nil, 0, fmt.Errorf("failed to count comment reports: %w", err)
	}

	rows := make([]AdminReportListItem, 0)
	if total == 0 {
		return rows, 0, nil
	}

	if err := applyFilter(base(m.mysql.WithContext(ctx))).
		Select(`cr.id, cr.comment_id, cr.article_id, a.title AS article_title,
			c.user_id AS comment_author_id, c.author_name AS comment_author_name, cu.handle AS comment_author_handle,
			c.content AS comment_content, c.status AS comment_status,
			cr.reporter_id, ru.display_name AS reporter_name, ru.handle AS reporter_handle,
			cr.reason, cr.status, cr.ip, cr.create_time, cr.update_time`).
		Order("cr.create_time DESC, cr.id DESC").
		Offset(filter.Offset).
		Limit(filter.Limit).
		Scan(&rows).Error; err != nil {
		if isCommentReportTableMissing(err) {
			return []AdminReportListItem{}, 0, nil
		}
		return nil, 0, fmt.Errorf("failed to list comment reports: %w", err)
	}
	return rows, total, nil
}

func isCommentReportTableMissing(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "error 1146") &&
		strings.Contains(message, "comment_report") &&
		strings.Contains(message, "doesn't exist")
}

func (m *commentModel) ResolvePendingCommentReports(ctx context.Context,
	commentID uint64, reportStatus string, commentStatus string, updateTime time.Time) error {

	return m.mysql.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&CommentReport{}).
			Where("comment_id = ? AND status = ?", commentID, ReportStatusPending).
			Updates(map[string]any{
				"status":      reportStatus,
				"update_time": updateTime,
			})
		if result.Error != nil {
			return fmt.Errorf("failed to resolve comment reports: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}

		if err := tx.Model(&Comment{}).
			Where("id = ?", commentID).
			Updates(map[string]any{
				"status":      commentStatus,
				"update_time": updateTime,
			}).Error; err != nil {
			return fmt.Errorf("failed to update reported comment status: %w", err)
		}
		return nil
	})
}

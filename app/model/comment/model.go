package comment

import (
	"context"
	"time"

	"gorm.io/gorm"
)

type Model interface {
	CreateComment(ctx context.Context, newComment *Comment) error
	CreateCommentWithModerationAudit(ctx context.Context, item *Comment, audit *CommentModerationAudit) error
	CreateModerationAudits(ctx context.Context, items []CommentModerationAudit) error
	GetModerationAuditByID(ctx context.Context, id uint64) (*CommentModerationAudit, error)
	GetLatestModerationAuditByCommentID(ctx context.Context, commentID uint64) (*CommentModerationAudit, error)
	CreateModerationFeedback(ctx context.Context, item *CommentModerationFeedback) error
	ReviewComment(ctx context.Context, id uint64, status string, updateTime time.Time,
		feedback *CommentModerationFeedback) error
	ResolveModerationFeedbackPolicy(ctx context.Context, contentHash, relationFingerprint string) (*ModerationFeedbackPolicy, error)
	GetCommentByID(ctx context.Context, id uint64) (*Comment, error)
	GetCommentsByIDs(ctx context.Context, ids []uint64) ([]*Comment, error)
	ListApprovedByArticleID(ctx context.Context, articleID uint64) ([]ListItem, error)
	ListApprovedArticleIDsByUserID(ctx context.Context, userID uint64) ([]uint64, error)
	ListComments(ctx context.Context, filter AdminListFilter) ([]AdminListItem, int64, error)
	GetAdminCommentByID(ctx context.Context, id uint64) (*AdminListItem, error)
	CreateCommentReport(ctx context.Context, report *CommentReport, threshold int64, updateTime time.Time) (int64, bool, error)
	GetCommentReportByCommentAndReporter(ctx context.Context, commentID uint64, reporterID uint64) (*CommentReport, error)
	ListReportedCommentIDsByReporter(ctx context.Context, commentIDs []uint64, reporterID uint64) ([]uint64, error)
	ListCommentReports(ctx context.Context, filter AdminReportListFilter) ([]AdminReportListItem, int64, error)
	ResolvePendingCommentReports(ctx context.Context, commentID uint64, reportStatus string, commentStatus string, updateTime time.Time) error
	UpdateCommentStatus(ctx context.Context, id uint64, status string, updateTime time.Time) error
	DeleteComment(ctx context.Context, id uint64) error
	DeleteComments(ctx context.Context, ids []uint64) error
}

type commentModel struct {
	mysql *gorm.DB
}

func NewModel(mysql *gorm.DB) Model {
	return &commentModel{mysql: mysql}
}

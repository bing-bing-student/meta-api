package comment

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	ModerationAuditSourceLiveComment     = "live_comment"
	ModerationAuditSourceAdminSimulation = "admin_simulation"
	ModerationFeedbackStateConfirmed     = "confirmed"
)

// CommentModerationAudit is an immutable snapshot of one moderation run.
// Source separates production traffic from simulations without preventing
// confirmed simulation feedback from contributing to policy decisions.
type CommentModerationAudit struct {
	ID                  uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	Source              string    `gorm:"column:source;type:varchar(32);NOT NULL;index:idx_moderation_audit_source_time,priority:1"`
	BatchID             string    `gorm:"column:batch_id;type:varchar(64);NOT NULL;default:'';index"`
	CommentID           uint64    `gorm:"column:comment_id;NOT NULL;default:0;index"`
	UserID              uint64    `gorm:"column:user_id;NOT NULL;default:0;index"`
	ArticleID           uint64    `gorm:"column:article_id;NOT NULL;default:0;index"`
	OperatorID          uint64    `gorm:"column:operator_id;NOT NULL;default:0;index"`
	Content             string    `gorm:"column:content;type:text;NOT NULL"`
	ContentHash         string    `gorm:"column:content_hash;type:char(64);NOT NULL;index"`
	RelationFingerprint string    `gorm:"column:relation_fingerprint;type:char(64);NOT NULL;default:'';index"`
	PolicyVersion       string    `gorm:"column:policy_version;type:varchar(80);NOT NULL;index"`
	RequestSnapshot     string    `gorm:"column:request_snapshot;type:longtext;NOT NULL"`
	ResultSnapshot      string    `gorm:"column:result_snapshot;type:longtext;NOT NULL"`
	Status              string    `gorm:"column:status;type:varchar(20);NOT NULL;index"`
	RiskScore           int       `gorm:"column:risk_score;NOT NULL;default:0"`
	RiskProbability     float64   `gorm:"column:risk_probability;type:decimal(8,7);NOT NULL;default:0"`
	Decision            string    `gorm:"column:decision;type:varchar(80);NOT NULL;default:''"`
	CreateTime          time.Time `gorm:"column:create_time;NOT NULL;index:idx_moderation_audit_source_time,priority:2"`
}

type CommentModerationFeedback struct {
	ID                 uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	AuditID            uint64    `gorm:"column:audit_id;NOT NULL;uniqueIndex:idx_moderation_feedback_audit_operator,priority:1;index"`
	OperatorID         uint64    `gorm:"column:operator_id;NOT NULL;uniqueIndex:idx_moderation_feedback_audit_operator,priority:2;index"`
	ExpectedStatus     string    `gorm:"column:expected_status;type:varchar(20);NOT NULL;index"`
	ExpectedCategory   string    `gorm:"column:expected_category;type:varchar(64);NOT NULL;default:'';index"`
	RelationCorrection string    `gorm:"column:relation_correction;type:longtext;NOT NULL"`
	Note               string    `gorm:"column:note;type:varchar(500);NOT NULL;default:''"`
	State              string    `gorm:"column:state;type:varchar(20);NOT NULL;index"`
	CreateTime         time.Time `gorm:"column:create_time;NOT NULL;index"`
	UpdateTime         time.Time `gorm:"column:update_time;NOT NULL"`
}

func (CommentModerationAudit) TableName() string {
	return "comment_moderation_audit"
}

func (CommentModerationFeedback) TableName() string {
	return "comment_moderation_feedback"
}

type ModerationFeedbackPolicy struct {
	ExpectedStatus    string
	ExpectedCategory  string
	Support           int64
	Total             int64
	Conflicts         int64
	SimulationSupport int64
	LiveSupport       int64
	RequiredSupport   int64
	Consensus         bool
	Applicable        bool
	ExactContent      bool
}

func (m *commentModel) CreateCommentWithModerationAudit(ctx context.Context, item *Comment,
	audit *CommentModerationAudit,
) error {
	return m.mysql.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(item).Error; err != nil {
			return fmt.Errorf("failed to create comment: %w", err)
		}
		if audit == nil {
			return nil
		}
		if err := tx.Create(audit).Error; err != nil {
			return fmt.Errorf("failed to create comment moderation audit: %w", err)
		}
		return nil
	})
}

func (m *commentModel) CreateModerationAudits(ctx context.Context, items []CommentModerationAudit) error {
	if len(items) == 0 {
		return nil
	}
	if err := m.mysql.WithContext(ctx).Create(&items).Error; err != nil {
		return fmt.Errorf("failed to create moderation audits: %w", err)
	}
	return nil
}

func (m *commentModel) GetModerationAuditByID(ctx context.Context, id uint64) (*CommentModerationAudit, error) {
	var item CommentModerationAudit
	if err := m.mysql.WithContext(ctx).First(&item, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (m *commentModel) GetLatestModerationAuditByCommentID(ctx context.Context,
	commentID uint64,
) (*CommentModerationAudit, error) {
	var item CommentModerationAudit
	if err := m.mysql.WithContext(ctx).
		Where("comment_id = ? AND source = ?", commentID, ModerationAuditSourceLiveComment).
		Order("create_time DESC, id DESC").First(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (m *commentModel) CreateModerationFeedback(ctx context.Context, item *CommentModerationFeedback) error {
	if err := m.mysql.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "audit_id"}, {Name: "operator_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"expected_status", "expected_category", "relation_correction", "note", "state", "update_time",
		}),
	}).Create(item).Error; err != nil {
		return fmt.Errorf("failed to create moderation feedback: %w", err)
	}
	return nil
}

func (m *commentModel) ReviewComment(ctx context.Context, id uint64, status string, updateTime time.Time,
	feedback *CommentModerationFeedback,
) error {
	return m.mysql.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&Comment{}).Where("id = ?", id).Updates(map[string]any{
			"status": status, "update_time": updateTime,
		})
		if result.Error != nil {
			return fmt.Errorf("failed to update comment status: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		if feedback == nil {
			return nil
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "audit_id"}, {Name: "operator_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"expected_status", "expected_category", "relation_correction", "note", "state", "update_time",
			}),
		}).Create(feedback).Error; err != nil {
			return fmt.Errorf("failed to create moderation feedback: %w", err)
		}
		return nil
	})
}

func (m *commentModel) ResolveModerationFeedbackPolicy(ctx context.Context, contentHash,
	relationFingerprint string,
) (*ModerationFeedbackPolicy, error) {
	type policyRow struct {
		ExpectedStatus   string
		ExpectedCategory string
		Support          int64
	}
	query := func(column, value string, requiredSupport int64, exact bool) (*ModerationFeedbackPolicy, error) {
		if value == "" {
			return nil, nil
		}
		feedbackTable := (CommentModerationFeedback{}).TableName()
		auditTable := (CommentModerationAudit{}).TableName()
		base := m.mysql.WithContext(ctx).Table(feedbackTable+" AS f").
			Joins("JOIN "+auditTable+" AS a ON a.id = f.audit_id").
			Where("f.state = ? AND a."+column+" = ?", ModerationFeedbackStateConfirmed, value)
		var total int64
		if err := base.Session(&gorm.Session{}).Count(&total).Error; err != nil {
			return nil, err
		}
		var row policyRow
		err := base.Session(&gorm.Session{}).
			Select("f.expected_status, f.expected_category, COUNT(*) AS support").
			Group("f.expected_status, f.expected_category").
			Order("support DESC, MAX(f.update_time) DESC").
			Limit(1).Scan(&row).Error
		if err != nil {
			return nil, err
		}
		if row.Support == 0 {
			return nil, nil
		}
		type sourceCountRow struct {
			Source string
			Count  int64
		}
		var sourceCounts []sourceCountRow
		if err = m.mysql.WithContext(ctx).Table(feedbackTable+" AS f").
			Joins("JOIN "+auditTable+" AS a ON a.id = f.audit_id").
			Where("f.state = ? AND a."+column+" = ?", ModerationFeedbackStateConfirmed, value).
			Where("f.expected_status = ? AND f.expected_category = ?", row.ExpectedStatus, row.ExpectedCategory).
			Select("a.source, COUNT(*) AS count").Group("a.source").Scan(&sourceCounts).Error; err != nil {
			return nil, err
		}
		policy := &ModerationFeedbackPolicy{
			ExpectedStatus: row.ExpectedStatus, ExpectedCategory: row.ExpectedCategory,
			Support: row.Support, Total: total, Conflicts: total - row.Support,
			RequiredSupport: requiredSupport, Consensus: row.Support*2 > total,
			ExactContent: exact,
		}
		for _, source := range sourceCounts {
			switch source.Source {
			case ModerationAuditSourceAdminSimulation:
				policy.SimulationSupport = source.Count
			case ModerationAuditSourceLiveComment:
				policy.LiveSupport = source.Count
			}
		}
		policy.Applicable = policy.Consensus && policy.Support >= policy.RequiredSupport
		return policy, nil
	}
	exactPolicy, err := query("content_hash", contentHash, 1, true)
	if err != nil {
		return nil, err
	}
	if exactPolicy != nil && exactPolicy.Applicable {
		return exactPolicy, nil
	}
	relationPolicy, err := query("relation_fingerprint", relationFingerprint, 2, false)
	if err != nil {
		return nil, err
	}
	if relationPolicy != nil && relationPolicy.Applicable {
		return relationPolicy, nil
	}
	if exactPolicy != nil {
		return exactPolicy, nil
	}
	return relationPolicy, nil
}

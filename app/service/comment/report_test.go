package comment

import (
	"context"
	"testing"
	"time"

	"github.com/sony/sonyflake"
	"go.uber.org/zap"
	"gorm.io/gorm"

	commentModel "meta-api/app/model/comment"
	userModel "meta-api/app/model/user"
	"meta-api/common/types"
	appconfig "meta-api/config"
)

func TestResolveCommentReportAction(t *testing.T) {
	reportStatus, commentStatus, err := resolveCommentReportAction("accept")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reportStatus != commentModel.ReportStatusAccepted || commentStatus != commentModel.StatusRejected {
		t.Fatalf("unexpected accept action result: %s %s", reportStatus, commentStatus)
	}

	reportStatus, commentStatus, err = resolveCommentReportAction("reject")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reportStatus != commentModel.ReportStatusRejected || commentStatus != commentModel.StatusApproved {
		t.Fatalf("unexpected reject action result: %s %s", reportStatus, commentStatus)
	}

	if _, _, err = resolveCommentReportAction("unknown"); err == nil {
		t.Fatal("expected invalid action error")
	}
}

func TestUserReportCommentRejectsSelfReport(t *testing.T) {
	service := newReportTestService(&reportTestCommentModel{
		comment: &commentModel.Comment{
			ID:        10001,
			ArticleID: 20001,
			UserID:    30001,
			Status:    commentModel.StatusApproved,
		},
	}, &reportTestUserModel{
		user: &userModel.User{ID: 30001, SessionVersion: 1},
	})

	_, err := service.UserReportComment(context.Background(), &types.UserReportCommentRequest{
		CommentID:      "10001",
		UserID:         "30001",
		SessionVersion: 1,
	})
	if err != ErrCommentForbidden {
		t.Fatalf("expected forbidden self report, got %v", err)
	}
}

func TestUserReportCommentRejectsDuplicateReport(t *testing.T) {
	service := newReportTestService(&reportTestCommentModel{
		comment: &commentModel.Comment{
			ID:        10001,
			ArticleID: 20001,
			UserID:    30001,
			Status:    commentModel.StatusApproved,
		},
		existingReport: &commentModel.CommentReport{ID: 40001},
	}, &reportTestUserModel{
		user: &userModel.User{ID: 30002, SessionVersion: 1},
	})

	_, err := service.UserReportComment(context.Background(), &types.UserReportCommentRequest{
		CommentID:      "10001",
		UserID:         "30002",
		SessionVersion: 1,
	})
	if err != ErrCommentAlreadyReported {
		t.Fatalf("expected duplicate report error, got %v", err)
	}
}

func TestUserReportCommentMovesApprovedCommentToPendingAtThreshold(t *testing.T) {
	commentStore := &reportTestCommentModel{
		comment: &commentModel.Comment{
			ID:        10001,
			ArticleID: 20001,
			UserID:    30001,
			Status:    commentModel.StatusApproved,
		},
		reportCount:    3,
		movedToPending: true,
	}
	service := newReportTestService(commentStore, &reportTestUserModel{
		user: &userModel.User{ID: 30002, SessionVersion: 1},
	})

	response, err := service.UserReportComment(context.Background(), &types.UserReportCommentRequest{
		CommentID:      "10001",
		UserID:         "30002",
		SessionVersion: 1,
		Reason:         "恶意广告",
		ClientIP:       "127.0.0.1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if response.Status != commentModel.StatusPending {
		t.Fatalf("expected pending status, got %s", response.Status)
	}
	if response.ReportCount != 3 {
		t.Fatalf("expected report count 3, got %d", response.ReportCount)
	}
	if commentStore.createdReport == nil {
		t.Fatal("expected created report")
	}
	if commentStore.createdReport.Reason != "恶意广告" {
		t.Fatalf("unexpected report reason: %s", commentStore.createdReport.Reason)
	}
}

func TestUserGetCommentReportStatusReturnsReportedIDs(t *testing.T) {
	commentStore := &reportTestCommentModel{
		reportedIDs: []uint64{10001, 10003},
	}
	service := newReportTestService(commentStore, &reportTestUserModel{
		user: &userModel.User{ID: 30002, SessionVersion: 1},
	})

	response, err := service.UserGetCommentReportStatus(context.Background(), &types.UserGetCommentReportStatusRequest{
		CommentIDs:     []string{"10001", "10002", "10001", "10003"},
		UserID:         "30002",
		SessionVersion: 1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(commentStore.reportStatusQueryIDs) != 3 {
		t.Fatalf("expected deduped query ids, got %v", commentStore.reportStatusQueryIDs)
	}
	if commentStore.reportStatusReporterID != 30002 {
		t.Fatalf("expected reporter id 30002, got %d", commentStore.reportStatusReporterID)
	}
	if len(response.ReportedCommentIDs) != 2 ||
		response.ReportedCommentIDs[0] != "10001" ||
		response.ReportedCommentIDs[1] != "10003" {
		t.Fatalf("unexpected reported comment ids: %v", response.ReportedCommentIDs)
	}
}

func TestAdminGetCommentReportListUsesCommentQuery(t *testing.T) {
	now := time.Date(2026, 7, 29, 15, 30, 0, 0, time.Local)
	commentStore := &reportTestCommentModel{
		reportRows: []commentModel.AdminReportListItem{
			{
				ID:                40001,
				CommentID:         10001,
				ArticleID:         20001,
				ArticleTitle:      "测试文章",
				CommentAuthorName: "评论人",
				CommentContent:    "包含关键字的评论",
				CommentStatus:     commentModel.StatusPending,
				ReporterID:        30002,
				ReporterName:      "举报人",
				Status:            commentModel.ReportStatusPending,
				CreateTime:        now,
				UpdateTime:        now,
			},
		},
		reportTotal: 1,
	}
	service := newReportTestService(commentStore, &reportTestUserModel{})

	response, err := service.AdminGetCommentReportList(context.Background(), &types.AdminGetCommentReportListRequest{
		Page:         1,
		PageSize:     10,
		CommentQuery: "  10001  ",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if commentStore.reportListFilter.CommentQuery != "10001" {
		t.Fatalf("expected trimmed comment query, got %q", commentStore.reportListFilter.CommentQuery)
	}
	if commentStore.reportListFilter.Offset != 0 || commentStore.reportListFilter.Limit != 10 {
		t.Fatalf("unexpected pagination filter: %+v", commentStore.reportListFilter)
	}
	if response.Total != 1 || len(response.Rows) != 1 {
		t.Fatalf("unexpected response: %+v", response)
	}
	if response.Rows[0].CommentID != "10001" || response.Rows[0].CommentContent != "包含关键字的评论" {
		t.Fatalf("unexpected response row: %+v", response.Rows[0])
	}
}

func newReportTestService(commentStore commentModel.Model, userStore userModel.Model) *commentService {
	return &commentService{
		config: &appconfig.Config{
			CommentModerationConfig: &appconfig.CommentModerationConfig{ReportThreshold: 3},
		},
		logger:       zap.NewNop(),
		idGenerator:  sonyflake.NewSonyflake(sonyflake.Settings{}),
		commentModel: commentStore,
		userModel:    userStore,
	}
}

type reportTestCommentModel struct {
	comment        *commentModel.Comment
	existingReport *commentModel.CommentReport
	reportCount    int64
	movedToPending bool
	createdReport  *commentModel.CommentReport

	reportedIDs            []uint64
	reportStatusQueryIDs   []uint64
	reportStatusReporterID uint64

	reportRows       []commentModel.AdminReportListItem
	reportTotal      int64
	reportListFilter commentModel.AdminReportListFilter
}

func (m *reportTestCommentModel) CreateComment(ctx context.Context, newComment *commentModel.Comment) error {
	return nil
}

func (m *reportTestCommentModel) GetCommentByID(ctx context.Context, id uint64) (*commentModel.Comment, error) {
	if m.comment == nil || m.comment.ID != id {
		return nil, gorm.ErrRecordNotFound
	}
	return m.comment, nil
}

func (m *reportTestCommentModel) GetCommentsByIDs(ctx context.Context, ids []uint64) ([]*commentModel.Comment, error) {
	return nil, nil
}

func (m *reportTestCommentModel) ListApprovedByArticleID(ctx context.Context, articleID uint64) ([]commentModel.ListItem, error) {
	return nil, nil
}

func (m *reportTestCommentModel) ListApprovedParentsByArticleID(ctx context.Context, articleID uint64, offset int, limit int) ([]commentModel.ListItem, int64, error) {
	return nil, 0, nil
}

func (m *reportTestCommentModel) ListApprovedRepliesByParentID(ctx context.Context, parentID uint64, offset int, limit int) ([]commentModel.ListItem, int64, error) {
	return nil, 0, nil
}

func (m *reportTestCommentModel) ListComments(ctx context.Context, filter commentModel.AdminListFilter) ([]commentModel.AdminListItem, int64, error) {
	return nil, 0, nil
}

func (m *reportTestCommentModel) CreateCommentReport(ctx context.Context, report *commentModel.CommentReport, threshold int64, updateTime time.Time) (int64, bool, error) {
	m.createdReport = report
	if m.movedToPending && m.comment != nil {
		m.comment.Status = commentModel.StatusPending
	}
	return m.reportCount, m.movedToPending, nil
}

func (m *reportTestCommentModel) GetCommentReportByCommentAndReporter(ctx context.Context, commentID uint64, reporterID uint64) (*commentModel.CommentReport, error) {
	if m.existingReport == nil {
		return nil, gorm.ErrRecordNotFound
	}
	return m.existingReport, nil
}

func (m *reportTestCommentModel) ListReportedCommentIDsByReporter(ctx context.Context, commentIDs []uint64, reporterID uint64) ([]uint64, error) {
	m.reportStatusQueryIDs = append([]uint64{}, commentIDs...)
	m.reportStatusReporterID = reporterID
	return append([]uint64{}, m.reportedIDs...), nil
}

func (m *reportTestCommentModel) ListCommentReports(ctx context.Context, filter commentModel.AdminReportListFilter) ([]commentModel.AdminReportListItem, int64, error) {
	m.reportListFilter = filter
	return append([]commentModel.AdminReportListItem{}, m.reportRows...), m.reportTotal, nil
}

func (m *reportTestCommentModel) ResolvePendingCommentReports(ctx context.Context, commentID uint64, reportStatus string, commentStatus string, updateTime time.Time) error {
	return nil
}

func (m *reportTestCommentModel) UpdateCommentStatus(ctx context.Context, id uint64, status string, updateTime time.Time) error {
	return nil
}

func (m *reportTestCommentModel) DeleteComment(ctx context.Context, id uint64) error {
	return nil
}

func (m *reportTestCommentModel) DeleteComments(ctx context.Context, ids []uint64) error {
	return nil
}

type reportTestUserModel struct {
	user *userModel.User
}

func (m *reportTestUserModel) UpsertOAuthUser(ctx context.Context, user *userModel.User) (*userModel.User, error) {
	return nil, nil
}

func (m *reportTestUserModel) GetUserByID(ctx context.Context, id uint64) (*userModel.User, error) {
	if m.user == nil || m.user.ID != id {
		return nil, gorm.ErrRecordNotFound
	}
	return m.user, nil
}

func (m *reportTestUserModel) GetMaxNumericHandle(ctx context.Context) (uint64, error) {
	return 0, nil
}

func (m *reportTestUserModel) ListUsers(ctx context.Context, filter userModel.AdminListFilter) ([]userModel.AdminListItem, int64, error) {
	return nil, 0, nil
}

func (m *reportTestUserModel) UpdateCommentPermission(ctx context.Context, id uint64, disabled bool, reason string, disabledUntil *time.Time, updateTime time.Time) error {
	return nil
}

func (m *reportTestUserModel) IncrementSessionVersion(ctx context.Context, id uint64, updateTime time.Time) error {
	return nil
}

var _ commentModel.Model = (*reportTestCommentModel)(nil)
var _ userModel.Model = (*reportTestUserModel)(nil)

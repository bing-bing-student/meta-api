package comment

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/sony/sonyflake"
	"go.uber.org/zap"
	"gorm.io/gorm"

	articleModel "meta-api/app/model/article"
	commentModel "meta-api/app/model/comment"
	userModel "meta-api/app/model/user"
	"meta-api/common/types"
	appconfig "meta-api/config"
)

func TestUserAddCommentModerationRequestFlow(t *testing.T) {
	cases := []struct {
		name        string
		content     string
		wantStatus  string
		wantReasons bool
	}{
		{
			name:       "normal content approved",
			content:    "  这篇文章写得很好，受教了  ",
			wantStatus: commentModel.StatusApproved,
		},
		{
			name:        "contact content pending",
			content:     "可以加微信继续交流吗",
			wantStatus:  commentModel.StatusPending,
			wantReasons: true,
		},
		{
			name:        "gambling content rejected",
			content:     "这里有赌博博彩项目",
			wantStatus:  commentModel.StatusRejected,
			wantReasons: true,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			commentStore := &moderationRequestCommentModel{}
			service := newModerationRequestTestService(t, commentStore)

			response, err := service.UserAddComment(context.Background(), &types.UserAddCommentRequest{
				ArticleID:      "20002",
				Content:        tt.content,
				UserID:         "10001",
				SessionVersion: 1,
				ClientIP:       "127.0.0.1",
			})
			if err != nil {
				t.Fatalf("unexpected add comment error: %v", err)
			}
			if response.Status != tt.wantStatus {
				t.Fatalf("expected response status %s, got %+v", tt.wantStatus, response)
			}
			if commentStore.created == nil {
				t.Fatal("expected created comment")
			}
			if commentStore.created.Status != tt.wantStatus {
				t.Fatalf("expected persisted status %s, got %+v", tt.wantStatus, commentStore.created)
			}
			if commentStore.created.Content != strings.TrimSpace(tt.content) {
				t.Fatalf("expected trimmed content %q, got %q", strings.TrimSpace(tt.content), commentStore.created.Content)
			}
			reasons := decodeCommentModerationReasons(commentStore.created.ModerationReasons)
			if tt.wantReasons && len(reasons) == 0 {
				t.Fatalf("expected persisted moderation reasons, got empty value %q", commentStore.created.ModerationReasons)
			}
			if !tt.wantReasons && len(reasons) != 0 {
				t.Fatalf("expected no persisted moderation reasons, got %v", reasons)
			}
			responseID, err := strconv.ParseUint(response.ID, 10, 64)
			if err != nil {
				t.Fatalf("expected numeric response id, got %q", response.ID)
			}
			if responseID != commentStore.created.ID {
				t.Fatalf("expected response id %d, got %d", commentStore.created.ID, responseID)
			}
		})
	}
}

func TestAdminCommentModerationReasons(t *testing.T) {
	if got := formatCommentModerationReason("lexicon:sensitive:review:g点"); got != "敏感词库命中：g点（敏感内容，待人工复核）" {
		t.Fatalf("unexpected sensitive lexicon reason label: %s", got)
	}

	reasons := []string{"structure:contact:review:contact", "lexicon:gambling:block:博彩"}
	wantReasons := []string{
		"结构规则命中：联系方式特征（联系方式，待人工复核）",
		"敏感词库命中：博彩（赌博风险，建议拒绝）",
	}
	pendingItem := toAdminCommentItem(commentModel.AdminListItem{
		ID:                10001,
		Status:            commentModel.StatusPending,
		ModerationReasons: encodeCommentModerationReasons(reasons),
	})
	if strings.Join(pendingItem.ModerationReasons, "\n") != strings.Join(wantReasons, "\n") {
		t.Fatalf("expected moderation reasons %v, got %+v", wantReasons, pendingItem)
	}

	approvedItem := toAdminCommentItem(commentModel.AdminListItem{
		ID:                10002,
		Status:            commentModel.StatusApproved,
		ModerationReasons: encodeCommentModerationReasons(reasons),
	})
	if len(approvedItem.ModerationReasons) != 0 {
		t.Fatalf("expected approved comment reasons to be hidden, got %+v", approvedItem)
	}
}

func newModerationRequestTestService(t testing.TB, commentStore *moderationRequestCommentModel) *commentService {
	t.Helper()
	return &commentService{
		config: &appconfig.Config{
			RateLimitConfig: &appconfig.RateLimitConfig{
				CommentSubmit: appconfig.CommentSubmitRateLimitConfig{Disabled: true},
			},
			CommentModerationConfig: newTestCommentModerationConfig(t),
		},
		logger:       zap.NewNop(),
		idGenerator:  sonyflake.NewSonyflake(sonyflake.Settings{}),
		commentModel: commentStore,
		articleModel: &moderationRequestArticleModel{},
		userModel:    &moderationRequestUserModel{},
	}
}

type moderationRequestCommentModel struct {
	commentModel.Model
	created *commentModel.Comment
}

func (m *moderationRequestCommentModel) CreateComment(ctx context.Context, newComment *commentModel.Comment) error {
	copied := *newComment
	m.created = &copied
	return nil
}

func (m *moderationRequestCommentModel) GetCommentByID(ctx context.Context, id uint64) (*commentModel.Comment, error) {
	return nil, gorm.ErrRecordNotFound
}

type moderationRequestArticleModel struct {
	articleModel.Model
}

func (m *moderationRequestArticleModel) GetArticleDetailByID(ctx context.Context, id uint64) (*articleModel.Detail, error) {
	if id != 20002 {
		return nil, gorm.ErrRecordNotFound
	}
	return &articleModel.Detail{ID: id, Title: "测试文章"}, nil
}

type moderationRequestUserModel struct {
	userModel.Model
}

func (m *moderationRequestUserModel) GetUserByID(ctx context.Context, id uint64) (*userModel.User, error) {
	if id != 10001 {
		return nil, gorm.ErrRecordNotFound
	}
	return &userModel.User{
		ID:             id,
		DisplayName:    "评论用户",
		SessionVersion: 1,
	}, nil
}

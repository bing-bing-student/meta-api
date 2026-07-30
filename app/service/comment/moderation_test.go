package comment

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	commentModel "meta-api/app/model/comment"
	appconfig "meta-api/config"
)

func TestNormalizeCommentModerationText(t *testing.T) {
	got := normalizeCommentModerationText(" ＶＸ\u200b  赚钱 \n")
	if got != "vx 赚钱" {
		t.Fatalf("unexpected normalized text: %q", got)
	}
	if compact := compactCommentModerationText(got); compact != "vx赚钱" {
		t.Fatalf("unexpected compact text: %q", compact)
	}
	view := newCommentModerationTextView("可以去yue炮吗")
	if view.PinyinFolded != "可以去约炮吗" {
		t.Fatalf("expected mixed pinyin folded view, got %+v", view)
	}
}

func TestModerateCommentApprovesNormalContent(t *testing.T) {
	service := newModerationTestService(t, newTestCommentModerationConfig(t))

	result := service.moderateComment(context.Background(), commentModerationInput{
		CommentID: 1,
		UserID:    10001,
		ArticleID: 20002,
		ClientIP:  "127.0.0.1",
		Content:   "这篇文章写得很好，受教了",
		Now:       time.Unix(1000, 0),
	})

	if result.Status != commentModel.StatusApproved {
		t.Fatalf("expected approved status, got %+v", result)
	}
}

func TestModerateCommentUsesGoSWDBuiltinLexiconStrictly(t *testing.T) {
	service := newModerationTestService(t, newTestCommentModerationConfig(t))

	cases := []struct {
		name       string
		content    string
		wantStatus string
		wantPrefix string
	}{
		{
			name:       "gambling block",
			content:    "这里有赌博博彩项目",
			wantStatus: commentModel.StatusRejected,
			wantPrefix: "lexicon:gambling:block:",
		},
		{
			name:       "drug block",
			content:    "出售毒品是不允许的",
			wantStatus: commentModel.StatusRejected,
			wantPrefix: "lexicon:drugs:block:",
		},
		{
			name:       "political review",
			content:    "法轮功相关内容",
			wantStatus: commentModel.StatusPending,
			wantPrefix: "lexicon:political:review:",
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			result := service.moderateComment(context.Background(), commentModerationInput{
				CommentID: 1,
				UserID:    10001,
				ArticleID: 20002,
				ClientIP:  "127.0.0.1",
				Content:   tt.content,
				Now:       time.Unix(1000, 0),
			})
			if result.Status != tt.wantStatus {
				t.Fatalf("expected status %s, got %+v", tt.wantStatus, result)
			}
			if !containsReasonPrefix(result.Reasons, tt.wantPrefix) {
				t.Fatalf("expected reason prefix %q, got %+v", tt.wantPrefix, result.Reasons)
			}
		})
	}
}

func TestModerateCommentKeepsCustomWordsEmptyByDefault(t *testing.T) {
	cfg := newTestCommentModerationConfig(t)
	if len(cfg.Lexicon.CustomWords.Block) != 0 || len(cfg.Lexicon.CustomWords.Review) != 0 {
		t.Fatalf("expected empty custom words for first version, got %+v", cfg.Lexicon.CustomWords)
	}
}

func TestModerateCommentSupportsCustomWordTuning(t *testing.T) {
	cfg := newTestCommentModerationConfig(t)
	cfg.Lexicon.CustomWords.Review = map[string][]string{
		"custom": {"本站漏审词"},
	}
	service := newModerationTestService(t, cfg)

	result := service.moderateComment(context.Background(), commentModerationInput{
		CommentID: 1,
		UserID:    10001,
		ArticleID: 20002,
		ClientIP:  "127.0.0.1",
		Content:   "这条评论包含本站漏审词",
		Now:       time.Unix(1000, 0),
	})

	if result.Status != commentModel.StatusPending {
		t.Fatalf("expected pending status, got %+v", result)
	}
	if !containsReasonPrefix(result.Reasons, "lexicon:custom:review:") {
		t.Fatalf("expected custom lexicon reason, got %+v", result.Reasons)
	}
}

func TestModerateCommentDetectsStructureSignals(t *testing.T) {
	service := newModerationTestService(t, newTestCommentModerationConfig(t))

	cases := []struct {
		name       string
		content    string
		wantStatus string
		wantPrefix string
	}{
		{
			name:       "script block",
			content:    "<script>alert(1)</script>",
			wantStatus: commentModel.StatusRejected,
			wantPrefix: "structure:script_injection:block:",
		},
		{
			name:       "contact review",
			content:    "可以加微信交流吗",
			wantStatus: commentModel.StatusPending,
			wantPrefix: "structure:contact:review:",
		},
		{
			name:       "encoded url review",
			content:    "地址：aHR0cHM6Ly9saXViaW5nLnh5eg== 你们base64解码就能看到",
			wantStatus: commentModel.StatusPending,
			wantPrefix: "structure:decoded_url:review:https://liubing.xyz",
		},
		{
			name:       "low quality review",
			content:    "哈哈哈哈哈哈哈哈哈哈",
			wantStatus: commentModel.StatusPending,
			wantPrefix: "structure:text_quality:review:repeated",
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			result := service.moderateComment(context.Background(), commentModerationInput{
				CommentID: 1,
				UserID:    10001,
				ArticleID: 20002,
				ClientIP:  "127.0.0.1",
				Content:   tt.content,
				Now:       time.Unix(1000, 0),
			})
			if result.Status != tt.wantStatus {
				t.Fatalf("expected status %s, got %+v", tt.wantStatus, result)
			}
			if !containsReasonPrefix(result.Reasons, tt.wantPrefix) {
				t.Fatalf("expected reason prefix %q, got %+v", tt.wantPrefix, result.Reasons)
			}
		})
	}
}

func TestModerateCommentDetectsContextSignals(t *testing.T) {
	service := newModerationTestService(t, newTestCommentModerationConfig(t))

	cases := []struct {
		name       string
		content    string
		wantPrefix string
	}{
		{
			name:       "political context",
			content:    "美国灭亡",
			wantPrefix: "context:political:review:political_mobilization:",
		},
		{
			name:       "minor risk",
			content:    "未成年交友群，学生专属，扫码进",
			wantPrefix: "context:minor:review:minor_risk:",
		},
		{
			name:       "illegal privacy",
			content:    "出售个人身份证信息",
			wantPrefix: "context:illegal_privacy:review:illegal_privacy:",
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			result := service.moderateComment(context.Background(), commentModerationInput{
				CommentID: 1,
				UserID:    10001,
				ArticleID: 20002,
				ClientIP:  "127.0.0.1",
				Content:   tt.content,
				Now:       time.Unix(1000, 0),
			})
			if result.Status != commentModel.StatusPending && result.Status != commentModel.StatusRejected {
				t.Fatalf("expected moderated status, got %+v", result)
			}
			if !containsReasonPrefix(result.Reasons, tt.wantPrefix) {
				t.Fatalf("expected reason prefix %q, got %+v", tt.wantPrefix, result.Reasons)
			}
		})
	}
}

func TestModerateCommentBehaviorSignals(t *testing.T) {
	service := newModerationTestService(t, newTestCommentModerationConfig(t))
	input := commentModerationInput{
		CommentID: 1,
		UserID:    10001,
		ArticleID: 20002,
		ClientIP:  "127.0.0.1",
		Content:   "这是一条重复评论",
		Now:       time.Unix(1000, 0),
	}

	result := service.moderateCommentWithBehavior(context.Background(), input,
		func(context.Context, commentModerationInput, commentModerationTextView,
			appconfig.CommentModerationConfig) []commentModerationSignal {
			return []commentModerationSignal{commentModerationBehaviorSignal(commentModerationBehaviorState{
				DuplicateCount: 3,
			}, *newTestCommentModerationConfig(t))}
		})

	if result.Status != commentModel.StatusRejected {
		t.Fatalf("expected rejected duplicate behavior, got %+v", result)
	}
	if !containsReasonPrefix(result.Reasons, "behavior:duplicate_content:block:") {
		t.Fatalf("expected duplicate behavior reason, got %+v", result.Reasons)
	}
}

func TestFillCommentModerationDefaults(t *testing.T) {
	cfg := appconfig.CommentModerationConfig{}

	fillCommentModerationDefaults(&cfg)

	if cfg.Decision.Score.Pending != defaultCommentModerationPendingScore {
		t.Fatalf("unexpected pending score: %+v", cfg.Decision.Score)
	}
	if cfg.Decision.Score.Reject != defaultCommentModerationRejectScore {
		t.Fatalf("unexpected reject score: %+v", cfg.Decision.Score)
	}
	if !cfg.Lexicon.UseBuiltin || !cfg.Lexicon.StrictBuiltinMatch {
		t.Fatalf("expected strict builtin lexicon defaults, got %+v", cfg.Lexicon)
	}
	if len(cfg.Lexicon.CustomWords.Block) != 0 || len(cfg.Lexicon.CustomWords.Review) != 0 {
		t.Fatalf("expected custom words to stay empty by default, got %+v", cfg.Lexicon.CustomWords)
	}
}

func TestApplyCommentModerationConfigLayout(t *testing.T) {
	cfg := appconfig.CommentModerationConfig{}

	if cfg.Lexicon.Provider != "go_swd" || !cfg.Lexicon.UseBuiltin {
		t.Fatalf("expected go_swd builtin lexicon, got %+v", cfg.Lexicon)
	}
	if cfg.StructureRules["script_injection"].Level != "block" {
		t.Fatalf("expected default structure rules, got %+v", cfg.StructureRules)
	}
	if cfg.Decision.CategoryOverrides["gambling"].Level != "block" {
		t.Fatalf("expected strict category overrides, got %+v", cfg.Decision.CategoryOverrides)
	}
}

func TestBuildCommentModerationBehaviorKeys(t *testing.T) {
	keys := buildCommentModerationBehaviorKeys(10001, 20002, " 127.0.0.1 ", "hello")

	if !strings.HasPrefix(keys.user, "comment:moderation:behavior:user:") {
		t.Fatalf("unexpected user key: %s", keys.user)
	}
	if !strings.HasPrefix(keys.ip, "comment:moderation:behavior:ip:") {
		t.Fatalf("unexpected ip key: %s", keys.ip)
	}
	if !strings.HasPrefix(keys.duplicate, "comment:moderation:behavior:duplicate:") {
		t.Fatalf("unexpected duplicate key: %s", keys.duplicate)
	}
}

func newModerationTestService(t testing.TB, cfg *appconfig.CommentModerationConfig) *commentService {
	t.Helper()
	return &commentService{
		config: &appconfig.Config{
			CommentModerationConfig: cfg,
		},
		logger: zap.NewNop(),
	}
}

func containsCommentModerationMatch(matches []string, want string) bool {
	return slices.Contains(matches, want)
}

func containsReasonPrefix(reasons []string, prefix string) bool {
	for _, reason := range reasons {
		if strings.HasPrefix(reason, prefix) {
			return true
		}
	}
	return false
}

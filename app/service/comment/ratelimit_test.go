package comment

import (
	"strings"
	"testing"

	appconfig "meta-api/config"
)

func TestFillCommentSubmitRateLimitDefaults(t *testing.T) {
	cfg := appconfig.CommentSubmitRateLimitConfig{}

	fillCommentSubmitRateLimitDefaults(&cfg)

	if cfg.IP.Limit != defaultCommentSubmitIPLimit || cfg.IP.WindowSeconds != 600 {
		t.Fatalf("unexpected ip limit: %+v", cfg.IP)
	}
	if cfg.User.Limit != defaultCommentSubmitUserLimit || cfg.User.WindowSeconds != 300 {
		t.Fatalf("unexpected user limit: %+v", cfg.User)
	}
	if cfg.UserArticle.Limit != defaultCommentSubmitUserArticleLimit || cfg.UserArticle.WindowSeconds != 300 {
		t.Fatalf("unexpected user article limit: %+v", cfg.UserArticle)
	}
}

func TestFillCommentReportRateLimitDefaults(t *testing.T) {
	cfg := appconfig.CommentReportRateLimitConfig{}

	fillCommentReportRateLimitDefaults(&cfg)

	if cfg.IP.Limit != defaultCommentReportIPLimit || cfg.IP.WindowSeconds != 86400 {
		t.Fatalf("unexpected ip limit: %+v", cfg.IP)
	}
	if cfg.User.Limit != defaultCommentReportUserLimit || cfg.User.WindowSeconds != 86400 {
		t.Fatalf("unexpected user limit: %+v", cfg.User)
	}
	if cfg.IPComment.Limit != defaultCommentReportIPCommentLimit || cfg.IPComment.WindowSeconds != 86400 {
		t.Fatalf("unexpected ip comment limit: %+v", cfg.IPComment)
	}
}

func TestBuildCommentSubmitLimitKeys(t *testing.T) {
	keys := buildCommentSubmitLimitKeys(10001, 20002, " 127.0.0.1 ")

	if !strings.HasPrefix(keys.ip, "comment:rate-limit:submit:ip:") {
		t.Fatalf("unexpected ip key: %s", keys.ip)
	}
	if !strings.HasPrefix(keys.user, "comment:rate-limit:submit:user:") {
		t.Fatalf("unexpected user key: %s", keys.user)
	}
	if !strings.HasPrefix(keys.userArticle, "comment:rate-limit:submit:user-article:") {
		t.Fatalf("unexpected user article key: %s", keys.userArticle)
	}
	if keys.user == keys.userArticle || keys.ip == keys.user {
		t.Fatalf("limit keys should use separate dimensions: %+v", keys)
	}
}

func TestBuildCommentReportLimitKeys(t *testing.T) {
	keys := buildCommentReportLimitKeys(10001, 20002, " 127.0.0.1 ")

	if !strings.HasPrefix(keys.ip, "comment:rate-limit:report:ip:") {
		t.Fatalf("unexpected ip key: %s", keys.ip)
	}
	if !strings.HasPrefix(keys.user, "comment:rate-limit:report:user:") {
		t.Fatalf("unexpected user key: %s", keys.user)
	}
	if !strings.HasPrefix(keys.ipComment, "comment:rate-limit:report:ip-comment:") {
		t.Fatalf("unexpected ip comment key: %s", keys.ipComment)
	}
	if keys.user == keys.ipComment || keys.ip == keys.user {
		t.Fatalf("limit keys should use separate dimensions: %+v", keys)
	}
}

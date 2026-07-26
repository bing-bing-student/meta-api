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

package middlewares

import (
	"testing"
	"time"
)

func TestTimeoutForPathUsesOverridePrefix(t *testing.T) {
	got := timeoutForPath("/user/auth/oauth/github/callback", 3*time.Second, []TimeoutOverride{
		{Prefix: "/user/auth/oauth/", Timeout: 35 * time.Second},
	})

	if got != 35*time.Second {
		t.Fatalf("expected oauth timeout override, got %s", got)
	}
}

func TestTimeoutForPathUsesAdminArticleUpdateOverride(t *testing.T) {
	got := timeoutForPath("/admin/auth/article/update", 3*time.Second, []TimeoutOverride{
		{Prefix: "/user/auth/oauth/", Timeout: 35 * time.Second},
		{Prefix: "/admin/auth/article/update", Timeout: 35 * time.Second},
	})

	if got != 35*time.Second {
		t.Fatalf("expected admin article update timeout override, got %s", got)
	}
}

func TestTimeoutForPathFallsBack(t *testing.T) {
	got := timeoutForPath("/user/article/detail", 3*time.Second, []TimeoutOverride{
		{Prefix: "/user/auth/oauth/", Timeout: 12 * time.Second},
	})

	if got != 3*time.Second {
		t.Fatalf("expected fallback timeout, got %s", got)
	}
}

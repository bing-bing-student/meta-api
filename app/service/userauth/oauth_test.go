package userauth

import "testing"

func TestEnvOrConfigPrefersEnv(t *testing.T) {
	t.Setenv("OAUTH_GITHUB_CLIENT_ID", "  local-client-id  ")

	got := envOrConfig("OAUTH_GITHUB_CLIENT_ID", "config-client-id")
	if got != "local-client-id" {
		t.Fatalf("expected env value, got %q", got)
	}
}

func TestEnvOrConfigFallsBackToConfig(t *testing.T) {
	got := envOrConfig("OAUTH_GITHUB_REDIRECT_URI", "  https://example.com/callback  ")
	if got != "https://example.com/callback" {
		t.Fatalf("expected trimmed config value, got %q", got)
	}
}

func TestNormalizeRedirectPathRejectsExternalRedirects(t *testing.T) {
	cases := []string{
		"https://evil.example/path",
		"//evil.example/path",
		"/login?next=https://evil.example/path",
	}

	for _, tc := range cases {
		if _, err := normalizeRedirectPath(tc); err == nil {
			t.Fatalf("expected redirect %q to be rejected", tc)
		}
	}
}

package middlewares

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDumpSanitizedRequestRedactsSensitiveHeadersAndQuery(t *testing.T) {
	req := httptest.NewRequest(
		http.MethodPost,
		"/admin/verify-dynamic-code?code=123456&loginChallenge=challenge-secret&token=token-secret&keyword=markdown",
		nil,
	)
	req.Header.Set("Authorization", "Bearer access-secret")
	req.Header.Set("Cookie", "access_token=access-secret; refresh_token=refresh-secret")
	req.Header.Set("X-Revalidate-Secret", "revalidate-secret")
	req.Header.Set("X-Request-ID", "request-id-1")

	dump := dumpSanitizedRequest(req)

	for _, secret := range []string{
		"123456",
		"challenge-secret",
		"token-secret",
		"access-secret",
		"refresh-secret",
		"revalidate-secret",
	} {
		if strings.Contains(dump, secret) {
			t.Fatalf("request dump leaked sensitive value %q: %s", secret, dump)
		}
	}
	for _, expected := range []string{
		"keyword=markdown",
		"Authorization: [REDACTED]",
		"Cookie: [REDACTED]",
		"X-Revalidate-Secret: [REDACTED]",
		"X-Request-Id: request-id-1",
	} {
		if !strings.Contains(dump, expected) {
			t.Fatalf("request dump missing %q: %s", expected, dump)
		}
	}
}

func TestDumpSanitizedRequestDoesNotMutateOriginalRequest(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/admin/auth/article/list?token=token-secret", nil)
	req.Header.Set("Authorization", "Bearer access-secret")
	req.Header.Set("Cookie", "access_token=access-secret")

	_ = dumpSanitizedRequest(req)

	if got := req.URL.RawQuery; got != "token=token-secret" {
		t.Fatalf("expected original raw query to stay unchanged, got %q", got)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer access-secret" {
		t.Fatalf("expected original authorization header to stay unchanged, got %q", got)
	}
	if got := req.Header.Get("Cookie"); got != "access_token=access-secret" {
		t.Fatalf("expected original cookie header to stay unchanged, got %q", got)
	}
}

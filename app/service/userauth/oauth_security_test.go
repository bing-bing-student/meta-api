package userauth

import (
	"strings"
	"testing"
)

func TestNormalizeRedirectPath(t *testing.T) {
	t.Parallel()

	valid := map[string]string{
		"":                                       "/",
		"/":                                      "/",
		"/article-detail/123":                    "/article-detail/123",
		"/article-detail/123?reply=1#comments":   "/article-detail/123?reply=1#comments",
		"/%E6%B5%8B%E8%AF%95?from=oauth#comment": "/%E6%B5%8B%E8%AF%95?from=oauth#comment",
	}
	for input, expected := range valid {
		input, expected := input, expected
		t.Run("valid_"+input, func(t *testing.T) {
			t.Parallel()
			actual, err := normalizeRedirectPath(input)
			if err != nil {
				t.Fatalf("normalizeRedirectPath(%q) returned error: %v", input, err)
			}
			if actual != expected {
				t.Fatalf("normalizeRedirectPath(%q) = %q, want %q", input, actual, expected)
			}
		})
	}

	invalid := []string{
		"https://evil.example/",
		"//evil.example/",
		`/\evil.example`,
		`/\/evil.example`,
		"/%5Cevil.example",
		"/%2F%2Fevil.example",
		"relative/path",
		"javascript:alert(1)",
		"/safe?next=%5Cevil.example",
		"/%0D%0ALocation:%20https://evil.example",
	}
	for _, input := range invalid {
		input := input
		t.Run("invalid_"+input, func(t *testing.T) {
			t.Parallel()
			if actual, err := normalizeRedirectPath(input); err == nil {
				t.Fatalf("normalizeRedirectPath(%q) unexpectedly accepted %q", input, actual)
			}
		})
	}
}

func TestOAuthFlowBindingAndPKCE(t *testing.T) {
	t.Parallel()

	binding, err := generateOAuthFlowSecret()
	if err != nil {
		t.Fatalf("generateOAuthFlowSecret() error: %v", err)
	}
	if len(binding) != 43 {
		t.Fatalf("flow binding length = %d, want 43", len(binding))
	}
	if strings.ContainsAny(binding, "+/=") {
		t.Fatalf("flow binding is not unpadded base64url: %q", binding)
	}

	bindingHash := hashOAuthFlowBinding(binding)
	if !oauthFlowBindingMatches(bindingHash, binding) {
		t.Fatal("matching OAuth flow binding was rejected")
	}
	if oauthFlowBindingMatches(bindingHash, binding+"x") {
		t.Fatal("mismatched OAuth flow binding was accepted")
	}
	if oauthFlowBindingMatches("not-a-sha256-hash", binding) {
		t.Fatal("malformed OAuth flow binding hash was accepted")
	}

	// RFC 7636 Appendix B test vector.
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	expectedChallenge := "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	if actual := oauthPKCEChallenge(verifier); actual != expectedChallenge {
		t.Fatalf("oauthPKCEChallenge() = %q, want %q", actual, expectedChallenge)
	}
}

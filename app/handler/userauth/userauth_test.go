package userauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap/zaptest"

	"meta-api/common/types"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestOAuthCallbackBindsURIAndQuerySeparately(t *testing.T) {
	service := &fakeUserAuthService{}
	handler := NewHandler(zaptest.NewLogger(t), service)

	router := gin.New()
	router.GET("/user/auth/oauth/:provider/callback", handler.OAuthCallback)

	state := strings.Repeat("a", 64)
	request := httptest.NewRequest(http.MethodGet,
		"/user/auth/oauth/google/callback?code=oauth-code&state="+state, nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d body=%s", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Location"); got != "/article-detail/1" {
		t.Fatalf("expected redirect location /article-detail/1, got %q", got)
	}
	if service.callbackRequest == nil {
		t.Fatal("expected callback service to be called")
	}
	if service.callbackRequest.Provider != "google" {
		t.Fatalf("expected provider google, got %q", service.callbackRequest.Provider)
	}
	if service.callbackRequest.Code != "oauth-code" {
		t.Fatalf("expected code oauth-code, got %q", service.callbackRequest.Code)
	}
	if service.callbackRequest.State != state {
		t.Fatalf("expected state %q, got %q", state, service.callbackRequest.State)
	}
}

type fakeUserAuthService struct {
	callbackRequest *types.OAuthCallbackRequest
}

func (s *fakeUserAuthService) BuildOAuthLoginURL(context.Context, *types.OAuthLoginRequest) (string, error) {
	return "", nil
}

func (s *fakeUserAuthService) HandleOAuthCallback(_ context.Context,
	request *types.OAuthCallbackRequest) (*types.OAuthCallbackResponse, string, error) {

	s.callbackRequest = request
	return &types.OAuthCallbackResponse{
		RedirectPath: "/article-detail/1",
	}, "access-token", nil
}

func (s *fakeUserAuthService) GetCurrentUser(context.Context, string, int64) (*types.PublicUserInfo, error) {
	return nil, nil
}

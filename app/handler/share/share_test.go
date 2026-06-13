package share

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap/zaptest"

	shareService "meta-api/app/service/share"
	"meta-api/common/guard"
)

// share_test.go —— Handler HTTP 适配冒烟测试
//
// 不依赖真 Redis / RSA 密钥；只验证 Precheck / Consume 在不同 service Outcome
// 下能正确映射 HTTP 状态、JSON 字段，以及 query/header 防御。

func init() {
	gin.SetMode(gin.TestMode)
}

func TestPrecheck_MissingTargetID(t *testing.T) {
	svc := &fakeShareService{}
	h := NewHandler(zaptest.NewLogger(t), svc)

	status, _ := callPrecheck(t, h, "", []byte("GUAR\x00\x00\x00\x00"))
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", status)
	}
	if svc.precheckCalls != 0 {
		t.Fatalf("expected service NOT called when target_id missing, got %d", svc.precheckCalls)
	}
}

func TestPrecheck_TargetIDTooLong(t *testing.T) {
	svc := &fakeShareService{}
	h := NewHandler(zaptest.NewLogger(t), svc)

	tooLong := strings.Repeat("a", targetIDMaxLen+1)
	status, _ := callPrecheck(t, h, tooLong, []byte("GUAR\x00\x00\x00\x00"))
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400 for oversize target_id, got %d", status)
	}
	if svc.precheckCalls != 0 {
		t.Fatalf("expected service NOT called for oversize target_id, got %d", svc.precheckCalls)
	}
}

func TestPrecheck_AcceptIssuesToken(t *testing.T) {
	svc := &fakeShareService{
		precheckOutcome: &shareService.PrecheckOutcome{
			HTTPStatus: http.StatusOK,
			Code:       2000,
			Message:    "ok",
			Token:      strings.Repeat("a", 64),
			ExpiresIn:  120,
		},
	}
	h := NewHandler(zaptest.NewLogger(t), svc)

	status, body := callPrecheck(t, h, "abc123", []byte("GUAR\x00\x00\x00\x00"))
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	if !strings.Contains(body, `"token":"`+strings.Repeat("a", 64)+`"`) {
		t.Fatalf("expected token in body, got: %s", body)
	}
	if svc.precheckCalls != 1 {
		t.Fatalf("expected service.Precheck called 1x, got %d", svc.precheckCalls)
	}
}

func TestPrecheck_SilentRejectReturnsEmptyToken(t *testing.T) {
	// 静默拒：HTTPStatus 仍为 200，但 token 为空。
	// 此设计避免脚本通过状态码区分通过/拒绝。
	svc := &fakeShareService{
		precheckOutcome: &shareService.PrecheckOutcome{
			HTTPStatus: http.StatusOK,
			Code:       2000,
			Message:    "ok",
		},
	}
	h := NewHandler(zaptest.NewLogger(t), svc)

	status, body := callPrecheck(t, h, "abc123", []byte("GUAR\x00\x00\x00\x00"))
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	if !strings.Contains(body, `"token":""`) {
		t.Fatalf("expected empty token in body, got: %s", body)
	}
}

func TestPrecheck_RateLimited(t *testing.T) {
	svc := &fakeShareService{
		precheckOutcome: &shareService.PrecheckOutcome{
			HTTPStatus: http.StatusTooManyRequests,
			Code:       4290,
			Message:    "rate limited",
		},
	}
	h := NewHandler(zaptest.NewLogger(t), svc)

	status, _ := callPrecheck(t, h, "abc123", []byte("GUAR\x00\x00\x00\x00"))
	if status != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", status)
	}
}

func TestPrecheck_ServiceError(t *testing.T) {
	svc := &fakeShareService{precheckErr: errors.New("boom")}
	h := NewHandler(zaptest.NewLogger(t), svc)

	status, _ := callPrecheck(t, h, "abc123", []byte("GUAR\x00\x00\x00\x00"))
	if status != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", status)
	}
}

func TestConsume_MissingHeader(t *testing.T) {
	// 没有 X-Guard-Token：service 内部判定为 invalid，统一返回 401。
	svc := &fakeShareService{
		consumeOutcome: &shareService.ConsumeOutcome{
			HTTPStatus: http.StatusUnauthorized,
			Code:       4010,
			Message:    "invalid token",
		},
	}
	h := NewHandler(zaptest.NewLogger(t), svc)

	status, _ := callConsume(t, h, "")
	if status != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", status)
	}
}

func TestConsume_Hit(t *testing.T) {
	fp := strings.Repeat("a", 64)
	svc := &fakeShareService{
		consumeOutcome: &shareService.ConsumeOutcome{
			HTTPStatus:  http.StatusOK,
			Code:        2000,
			Message:     "ok",
			Fingerprint: fp,
		},
	}
	h := NewHandler(zaptest.NewLogger(t), svc)

	status, body := callConsume(t, h, strings.Repeat("b", 64))
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	if !strings.Contains(body, `"fingerprint":"`+fp+`"`) {
		t.Fatalf("expected fingerprint in body, got: %s", body)
	}
}

func TestConsume_Miss(t *testing.T) {
	svc := &fakeShareService{
		consumeOutcome: &shareService.ConsumeOutcome{
			HTTPStatus: http.StatusUnauthorized,
			Code:       4010,
			Message:    "invalid token",
		},
	}
	h := NewHandler(zaptest.NewLogger(t), svc)

	status, _ := callConsume(t, h, strings.Repeat("c", 64))
	if status != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", status)
	}
}

/* -------------------------------------------------------------------------- */
/*  辅助                                                                       */
/* -------------------------------------------------------------------------- */

func callPrecheck(t *testing.T, h Handler, targetID string, body []byte) (int, string) {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	url := "/user/share/precheck"
	if targetID != "" {
		url += "?" + targetIDQueryKey + "=" + targetID
	}
	req := httptest.NewRequest(http.MethodPost, url, strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/octet-stream")
	c.Request = req
	h.Precheck(c)

	status := c.Writer.Status()
	if w.Body.Len() > 0 {
		status = w.Code
	}
	return status, w.Body.String()
}

func callConsume(t *testing.T, h Handler, token string) (int, string) {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/user/share/consume", strings.NewReader(""))
	if token != "" {
		req.Header.Set(tokenHeader, token)
	}
	c.Request = req
	h.Consume(c)

	status := c.Writer.Status()
	if w.Body.Len() > 0 {
		status = w.Code
	}
	return status, w.Body.String()
}

/* -------------------------------------------------------------------------- */
/*  Fakes                                                                      */
/* -------------------------------------------------------------------------- */

type fakeShareService struct {
	precheckOutcome *shareService.PrecheckOutcome
	precheckErr     error
	consumeOutcome  *shareService.ConsumeOutcome
	consumeErr      error

	precheckCalls int
	consumeCalls  int
}

func (f *fakeShareService) Precheck(_ context.Context, _ *guard.RiskRequest) (*shareService.PrecheckOutcome, error) {
	f.precheckCalls++
	return f.precheckOutcome, f.precheckErr
}

func (f *fakeShareService) Consume(_ context.Context, _ string) (*shareService.ConsumeOutcome, error) {
	f.consumeCalls++
	return f.consumeOutcome, f.consumeErr
}

// 静态断言 fakeShareService 实现 share.Service 接口
var _ shareService.Service = (*fakeShareService)(nil)

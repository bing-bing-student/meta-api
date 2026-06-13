package viewlog

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	viewlogService "meta-api/app/service/viewlog"
	"meta-api/common/guard"
)

// dispatch_test.go —— Handler 分流冒烟测试
//
// 不依赖真 Redis / RSA 密钥；只验证 PostViewLog 入口仅接受 GUAR 信封。
//
// 主要路径：
//   - body 以 "GUAR" 开头 → 调用 fakeEngine.Evaluate
//   - body 不以 "GUAR" 开头 → 直接 400，不触碰 engine 与 service

func init() {
	gin.SetMode(gin.TestMode)
}

func TestDispatch_NewEnvelope(t *testing.T) {
	engine := &fakeEngine{decision: guard.DecisionAccept, reason: "ACCEPTED"}
	service := &fakeService{}
	h := NewHandler(zap.NewNop(), service, engine)

	body := append([]byte("GUAR"), make([]byte, 100)...)
	status, _, c := callPostViewLog(t, h, "10001", body)

	if status != http.StatusNoContent {
		t.Fatalf("expected 204, got %d ctxStatus=%d", status, c.Writer.Status())
	}
	if engine.calls != 1 {
		t.Fatalf("expected engine.Evaluate called 1x, got %d", engine.calls)
	}
	if service.ensureCalls != 1 || service.incCalls != 1 {
		t.Fatalf("expected EnsureArticleExists=1 Increment=1, got %d/%d", service.ensureCalls, service.incCalls)
	}
}

func TestDispatch_GuardSilent(t *testing.T) {
	// 静默拒：即便文章不存在也不应继续校验 ensure/increment，直接 204
	engine := &fakeEngine{decision: guard.DecisionSilent, reason: "L1_UA"}
	service := &fakeService{}
	h := NewHandler(zap.NewNop(), service, engine)

	body := append([]byte("GUAR"), make([]byte, 50)...)
	status, _, _ := callPostViewLog(t, h, "10001", body)

	if status != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", status)
	}
	if service.ensureCalls != 0 || service.incCalls != 0 {
		t.Fatalf("silent reject should NOT call ensure/increment, got %d/%d", service.ensureCalls, service.incCalls)
	}
}

func TestDispatch_NonEnvelopeReturns400(t *testing.T) {
	// 非 GUAR 信封一律 400，不应触碰 engine 或 service。
	engine := &fakeEngine{}
	service := &fakeService{}
	h := NewHandler(zap.NewNop(), service, engine)

	body := []byte(`{"token":"dGVzdA==","tz":"Asia/Shanghai"}`)
	status, _, _ := callPostViewLog(t, h, "10001", body)

	if status != http.StatusBadRequest {
		t.Fatalf("expected 400 for non-envelope body, got %d", status)
	}
	if engine.calls != 0 {
		t.Fatalf("expected engine NOT called for non-envelope body, got %d", engine.calls)
	}
	if service.ensureCalls != 0 || service.incCalls != 0 {
		t.Fatalf("expected service NOT called for non-envelope body, got ensure=%d inc=%d",
			service.ensureCalls, service.incCalls)
	}
}

/* -------------------------------------------------------------------------- */
/*  辅助                                                                       */
/* -------------------------------------------------------------------------- */

// callPostViewLog 调用 Handler.PostViewLog 并返回 (有效 status, body, ctx)。
//
// gin 的 `c.Status()` 不会立即把状态码写到底层 http.ResponseWriter（仅在
// 第一次 Write 时才 flush）。对于"只有 status / 无 body"的响应（典型如 204），
// httptest.ResponseRecorder.Code 一直停在默认 200。这里优先取
// c.Writer.Status() —— 它是 gin 内部记录的最终状态码。
func callPostViewLog(t *testing.T, h Handler, articleID string, body []byte) (int, string, *gin.Context) {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: articleID}}
	req := httptest.NewRequest(http.MethodPost, "/user/article/view-log/"+articleID, io.NopCloser(bytes.NewReader(body)))
	req.Header.Set("Content-Type", "application/octet-stream")
	c.Request = req
	h.PostViewLog(c)

	status := c.Writer.Status()
	// 若 ResponseRecorder 已经被 flush 过（写过 body 的场景），以它为准
	if w.Body.Len() > 0 {
		status = w.Code
	}
	return status, w.Body.String(), c
}

/* -------------------------------------------------------------------------- */
/*  Fakes                                                                      */
/* -------------------------------------------------------------------------- */

type fakeEngine struct {
	decision guard.Decision
	reason   string
	calls    int
}

func (f *fakeEngine) Evaluate(_ context.Context, _ *guard.RiskRequest) (*guard.Outcome, error) {
	f.calls++
	return &guard.Outcome{Decision: f.decision, Reason: f.reason, Score: 90}, nil
}

type fakeService struct {
	ensureCalls int
	incCalls    int
}

func (f *fakeService) EnsureArticleExists(_ context.Context, _ string) *viewlogService.Outcome {
	f.ensureCalls++
	return nil // 视为存在
}

func (f *fakeService) Increment(_ context.Context, _ string) {
	f.incCalls++
}

// 静态断言 fakeService 实现 viewlog.Service 接口
var _ viewlogService.Service = (*fakeService)(nil)

// 静态断言 fakeEngine 实现 guard.Engine 接口
var _ guard.Engine = (*fakeEngine)(nil)

package revalidator

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"go.uber.org/zap"
)

// waitFor 在最长 d 时间内轮询 cond，cond 为 true 立即返回 true，超时返回 false。
//
// revalidator 的对外 API 是 fire-and-forget，单测里无法直接 wait goroutine，
// 所以统一在断言路径上用这种轮询的方式确认副作用是否发生。
func waitFor(d time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return cond()
}

// startMockNuxt 启动一个简易的 httptest server，模拟 Nuxt 的 _revalidate 接口。
//
// 返回值除 server 外还包含三个观察点：
//   - hits 已收到的请求计数（atomic）；
//   - lastSecret 最近一次请求带的 secret；
//   - lastBody 最近一次请求 body。
func startMockNuxt(status int) (*httptest.Server, *atomic.Int64, *atomic.Value, *atomic.Value) {
	hits := &atomic.Int64{}
	lastSecret := &atomic.Value{}
	lastBody := &atomic.Value{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		lastSecret.Store(r.Header.Get("x-revalidate-secret"))
		body, _ := io.ReadAll(r.Body)
		lastBody.Store(string(body))
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"revalidated":true,"cleared":{}}`))
	}))
	return srv, hits, lastSecret, lastBody
}

// setEnvs 同时设置 endpoint / secret 两个环境变量，t.Setenv 会在用例结束后自动还原。
func setEnvs(t *testing.T, endpoint, secret string) {
	t.Helper()
	t.Setenv(envEndpoint, endpoint)
	t.Setenv(envSecret, secret)
}

func TestNew_DisabledWhenEndpointMissing(t *testing.T) {
	// 仅设 secret，不设 endpoint，期望禁用
	t.Setenv(envEndpoint, "")
	t.Setenv(envSecret, "any")
	c := New(zap.NewNop())
	if c.enabled() {
		t.Fatalf("expected disabled when endpoint empty, got enabled")
	}
}

func TestNew_DisabledWhenSecretMissing(t *testing.T) {
	// 仅设 endpoint，不设 secret，期望禁用
	t.Setenv(envEndpoint, "http://nuxt:3000/api/_revalidate")
	t.Setenv(envSecret, "")
	c := New(zap.NewNop())
	if c.enabled() {
		t.Fatalf("expected disabled when secret empty, got enabled")
	}
}

func TestNew_DefaultTimeoutApplied(t *testing.T) {
	setEnvs(t, "http://nuxt:3000/api/_revalidate", "s")
	c := New(zap.NewNop())
	if c.timeout != defaultTimeout {
		t.Fatalf("expected default timeout %s, got %s", defaultTimeout, c.timeout)
	}
}

func TestRevalidate_NoopWhenDisabled(t *testing.T) {
	srv, hits, _, _ := startMockNuxt(http.StatusOK)
	defer srv.Close()

	// 故意只给 endpoint，不给 secret，client 应处于禁用态
	t.Setenv(envEndpoint, srv.URL)
	t.Setenv(envSecret, "")
	c := New(zap.NewNop())
	c.Revalidate("/any")

	// 给一段时间确认确实没发起 HTTP
	time.Sleep(50 * time.Millisecond)
	if hits.Load() != 0 {
		t.Fatalf("expected 0 hits when disabled, got %d", hits.Load())
	}
}

func TestRevalidate_NoopWhenPathsEmpty(t *testing.T) {
	srv, hits, _, _ := startMockNuxt(http.StatusOK)
	defer srv.Close()

	setEnvs(t, srv.URL, "s")
	c := New(zap.NewNop())
	c.Revalidate()

	time.Sleep(50 * time.Millisecond)
	if hits.Load() != 0 {
		t.Fatalf("expected 0 hits when paths empty, got %d", hits.Load())
	}
}

func TestRevalidate_SendsExpectedRequest(t *testing.T) {
	srv, hits, lastSecret, lastBody := startMockNuxt(http.StatusOK)
	defer srv.Close()

	setEnvs(t, srv.URL, "topsecret")
	c := New(zap.NewNop())
	c.Revalidate("/article-detail/1", "/", "/tag?tagName=Go")

	if !waitFor(time.Second, func() bool { return hits.Load() == 1 }) {
		t.Fatalf("expected 1 hit, got %d", hits.Load())
	}
	if got, _ := lastSecret.Load().(string); got != "topsecret" {
		t.Fatalf("expected secret 'topsecret', got %q", got)
	}
	body, _ := lastBody.Load().(string)
	parsed := struct {
		Paths []string `json:"paths"`
	}{}
	if err := sonic.UnmarshalString(body, &parsed); err != nil {
		t.Fatalf("body is not valid JSON: %s err=%v", body, err)
	}
	want := []string{"/article-detail/1", "/", "/tag?tagName=Go"}
	if len(parsed.Paths) != len(want) {
		t.Fatalf("expected %d paths, got %d (body=%s)", len(want), len(parsed.Paths), body)
	}
	for i, p := range want {
		if parsed.Paths[i] != p {
			t.Fatalf("paths[%d] = %q, want %q", i, parsed.Paths[i], p)
		}
	}
}

func TestRevalidate_SwallowsNon2xx(t *testing.T) {
	srv, hits, _, _ := startMockNuxt(http.StatusUnauthorized)
	defer srv.Close()

	setEnvs(t, srv.URL, "s")
	c := New(zap.NewNop())
	// 关键约定：调用方拿不到错误，单测靠"调用未 panic / 无 deadlock"判定通过
	c.Revalidate("/x")

	if !waitFor(time.Second, func() bool { return hits.Load() == 1 }) {
		t.Fatalf("expected 1 hit even when server returns 401, got %d", hits.Load())
	}
}

func TestRevalidate_PathsCloneIsolatesCallerMutation(t *testing.T) {
	srv, hits, _, lastBody := startMockNuxt(http.StatusOK)
	defer srv.Close()

	setEnvs(t, srv.URL, "s")
	c := New(zap.NewNop())
	paths := []string{"/article-detail/1"}
	c.Revalidate(paths...)
	// 立刻篡改原 slice，验证不会影响异步发出的请求
	paths[0] = "/article-detail/MUTATED"

	if !waitFor(time.Second, func() bool { return hits.Load() == 1 }) {
		t.Fatalf("expected 1 hit, got %d", hits.Load())
	}
	body, _ := lastBody.Load().(string)
	if want := `"/article-detail/1"`; !contains(body, want) {
		t.Fatalf("body should still carry the original path, body=%s", body)
	}
}

func TestArticleDetailPath(t *testing.T) {
	if got := ArticleDetailPath("111"); got != "/article-detail/111" {
		t.Fatalf("got %q", got)
	}
}

func TestHomePath(t *testing.T) {
	if got := HomePath(); got != "/" {
		t.Fatalf("got %q", got)
	}
}

func TestTagPath_PlainAscii(t *testing.T) {
	if got := TagPath("Go"); got != "/tag?tagName=Go" {
		t.Fatalf("got %q", got)
	}
}

func TestTagPath_EscapesUnicodeAndSpace(t *testing.T) {
	// "Go 语言" 应被 url.QueryEscape 编码，避免拼接出非法 URL
	got := TagPath("Go 语言")
	want := "/tag?tagName=Go+%E8%AF%AD%E8%A8%80"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// contains 替代 strings.Contains 以保持测试文件 import 数量最小。
func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && (indexOf(s, sub) >= 0))
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

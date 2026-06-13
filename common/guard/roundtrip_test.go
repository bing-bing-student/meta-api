package guard

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"meta-api/pkg/keymanager"
)

// roundtrip_test.go —— Rust ↔ Go 协议互通测试
//
// 目的：验证 crypto-wasm 产出的二进制信封能被 meta-api/common/guard.Engine
// byte-for-byte 解读、解密、HMAC 校验通过、TLV 解析一致、最终判定 Accept。
//
// 触发方式：
//
//   GUARD_FIXTURE_BIN=/tmp/crypto-wasm-target/aarch64-apple-darwin/debug/examples/gen_envelope \
//     go test ./common/guard/... -run TestRoundtripFromRustExample -v
//
// 没有设置 GUARD_FIXTURE_BIN 时整个测试会 t.Skip，避免在 CI 缺 cargo 时挂掉。

func TestRoundtripFromRustExample(t *testing.T) {
	binPath := os.Getenv("GUARD_FIXTURE_BIN")
	if binPath == "" {
		t.Skip("GUARD_FIXTURE_BIN not set; skip Rust↔Go roundtrip test")
	}
	if _, err := os.Stat(binPath); err != nil {
		t.Fatalf("GUARD_FIXTURE_BIN points to %q which is unreadable: %v", binPath, err)
	}

	// ------- 1. 在测试进程内即时生成 RSA-2048 密钥对 -------
	// 不复用文件 keys/private_key.pem，避免污染本机开发环境。
	prov, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	pubDER, err := x509.MarshalPKIXPublicKey(&prov.PublicKey)
	if err != nil {
		t.Fatalf("MarshalPKIXPublicKey: %v", err)
	}
	pubB64 := base64.StdEncoding.EncodeToString(pubDER)

	// ------- 2. 调用 Rust example 生成信封 -------
	const targetID = "rt-target-001"
	const fingerprintHex = "11223344556677889900aabbccddeeff" +
		"0011223344556677889900aabbccddee"
	metaJSON := `{"tz":"Asia/Shanghai","lang":"zh-CN","screen":"1280x800",` +
		`"viewport":"1280x800","perfNav":"navigate"}`

	// 行为流：构造 4 条事件覆盖 mousemove / click，模拟正常浏览
	behavior := buildSyntheticBehavior()
	behaviorB64 := base64.StdEncoding.EncodeToString(behavior)

	stdinJSON := buildExampleInputJSON(pubB64, byte(SceneViewLog), fingerprintHex,
		targetID, metaJSON, behaviorB64)

	envelope, buildHashHex, err := runExample(binPath, stdinJSON)
	if err != nil {
		t.Fatalf("run example: %v", err)
	}
	if len(envelope) < envMinLen {
		t.Fatalf("envelope too short: got %d bytes", len(envelope))
	}
	if !bytes.Equal(envelope[:4], envelopeMagic[:]) {
		t.Fatalf("envelope magic mismatch: got %x", envelope[:4])
	}
	if envelope[envSceneOffset] != byte(SceneViewLog) {
		t.Fatalf("scene byte mismatch: got 0x%x", envelope[envSceneOffset])
	}

	// ------- 3. 构造 Engine（注入测试 keymanager + fake store） -------
	logger := zaptest.NewLogger(t)
	km := keymanager.NewForTest(prov)
	store := newFakeStore()
	registry := NewBuildHashRegistry()
	if err := registry.RegisterFromHex(buildHashHex, time.Time{}); err != nil {
		t.Fatalf("register build hash %q: %v", buildHashHex, err)
	}
	eng, err := NewEngine(EngineConfig{
		KeyManager:        km,
		Store:             store,
		Logger:            logger,
		BuildHashes:       registry,
		SkipHMACWhenEmpty: false,
	})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	// ------- 4. 调用 Evaluate -------
	req := &RiskRequest{
		Scene:     SceneViewLog,
		TargetID:  targetID,
		RawBody:   envelope,
		ClientIP:  "127.0.0.1",
		UserAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36",
		Lang:      "zh-CN",
		Screen:    "1280x800",
		PerfNav:   "navigate",
		SecFetch: SecFetchHeaders{
			Mode:           "cors",
			Site:           "same-origin",
			Dest:           "empty",
			AcceptLanguage: "zh-CN,zh;q=0.9",
		},
	}
	out, err := eng.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatalf("Evaluate returned err: %v", err)
	}
	if out.Decision != DecisionAccept {
		t.Fatalf("expected Accept, got Decision=%d Reason=%q Score=%d",
			out.Decision, out.Reason, out.Score)
	}
	if out.Fingerprint != fingerprintHex {
		t.Fatalf("fingerprint mismatch: want %q got %q", fingerprintHex, out.Fingerprint)
	}

	// ------- 5. 二次相同信封：nonce 必须命中重放 -------
	out2, err := eng.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatalf("Evaluate (replay) returned err: %v", err)
	}
	if out2.Decision == DecisionAccept {
		t.Fatalf("expected replay reject, got Accept")
	}
	if !strings.Contains(out2.Reason, ReasonNonceReplay) {
		t.Fatalf("expected NONCE_REPLAY, got %q", out2.Reason)
	}
}

/* -------------------------------------------------------------------------- */
/*  辅助：构造 example 入参 / 解析 example 产出                                */
/* -------------------------------------------------------------------------- */

// buildExampleInputJSON 拼一份扁平 JSON，与 examples/gen_envelope.rs 内的
// minimal parser 期望的字段一致：
//
//	public_key_b64 / scene / fingerprint_hex / target_id / meta(object) / behavior_b64
//
// 用 fmt.Sprintf 而不是 encoding/json：避免 json marshal 把 meta 字段二次转义；
// example 端直接把 meta 当原始 JSON 子串塞进信封，必须保持原始字符。
func buildExampleInputJSON(pubB64 string, scene byte, fpHex, targetID, metaJSON, behaviorB64 string) string {
	return fmt.Sprintf(
		`{"public_key_b64":%q,"scene":%d,"fingerprint_hex":%q,"target_id":%q,"meta":%s,"behavior_b64":%q}`,
		pubB64, scene, fpHex, targetID, metaJSON, behaviorB64,
	)
}

// runExample 执行 cargo example 二进制：
//   - stdin 喂入 JSON 配置
//   - stdout 收原始信封字节
//   - stderr 末尾解析 BUILD_HASH=xxxx 行
func runExample(binPath, stdinJSON string) ([]byte, string, error) {
	cmd := exec.Command(binPath)
	cmd.Stdin = strings.NewReader(stdinJSON)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, "", fmt.Errorf("example exec failed: %w; stderr=%s", err, stderr.String())
	}
	bh, err := parseBuildHashFromStderr(stderr.String())
	if err != nil {
		return nil, "", fmt.Errorf("parse build hash: %w; stderr=%s", err, stderr.String())
	}
	return stdout.Bytes(), bh, nil
}

func parseBuildHashFromStderr(s string) (string, error) {
	const prefix = "BUILD_HASH="
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		hexStr := strings.TrimPrefix(line, prefix)
		if _, err := hex.DecodeString(hexStr); err != nil {
			return "", fmt.Errorf("invalid hex %q: %w", hexStr, err)
		}
		if len(hexStr) != fieldBuildHashLen*2 {
			return "", fmt.Errorf("expected %d hex chars, got %d", fieldBuildHashLen*2, len(hexStr))
		}
		return hexStr, nil
	}
	return "", errors.New("BUILD_HASH= line not found in stderr")
}

// buildSyntheticBehavior 构造一段 6B/event 的行为流（与 crypto-wasm/src/behavior.rs 对齐）。
//
// 共 6 条事件：mousemove×4 + click×2，dt/dx/dy 设置为合理范围，
// 让后端 behaviorEvaluator 不至于因"事件不足"或"无点击"扣分到阈值以下。
func buildSyntheticBehavior() []byte {
	const evtLen = 6
	out := make([]byte, 0, evtLen*6)
	add := func(t byte, dt uint16, dx, dy int8, meta byte) {
		var b [6]byte
		b[0] = t
		binary.BigEndian.PutUint16(b[1:3], dt)
		b[3] = byte(dx)
		b[4] = byte(dy)
		b[5] = meta
		out = append(out, b[:]...)
	}
	const (
		mousemove byte = 0x10
		click     byte = 0x13
	)
	add(mousemove, 50, 5, 3, 0)
	add(mousemove, 50, 4, 2, 0)
	add(mousemove, 60, 6, 4, 0)
	add(mousemove, 80, 5, 3, 0)
	add(click, 200, 0, 0, 0)
	add(click, 1000, 0, 0, 0)
	return out
}

/* -------------------------------------------------------------------------- */
/*  Fake Store: 内存模拟 Redis 行为                                            */
/* -------------------------------------------------------------------------- */

// fakeStore 在测试进程内模拟 Redis 的 SetNX / Incr 语义。
//
// 单元测试不引入真 Redis 是为了：
//   - 避免本机依赖；
//   - 保证测试纯函数式可重复（当前进程清空后状态归零）。
type fakeStore struct {
	nonces map[string]struct{}
	rates  map[string]int64
	dedup  map[string]struct{}
	tokens map[string]string
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		nonces: make(map[string]struct{}),
		rates:  make(map[string]int64),
		dedup:  make(map[string]struct{}),
		tokens: make(map[string]string),
	}
}

func (f *fakeStore) NonceTrySet(_ context.Context, scene Scene, nonce []byte, _ time.Duration) (bool, error) {
	key := fmt.Sprintf("%s:%x", scene.String(), nonce)
	if _, ok := f.nonces[key]; ok {
		return false, nil
	}
	f.nonces[key] = struct{}{}
	return true, nil
}

func (f *fakeStore) IncrCheckRate(_ context.Context, key string, _ time.Duration, threshold int64) (bool, error) {
	f.rates[key]++
	return f.rates[key] > threshold, nil
}

func (f *fakeStore) DedupTrySet(_ context.Context, scene Scene, fpHex, targetID string, _ time.Duration) (bool, error) {
	key := scene.String() + ":" + fpHex + ":" + targetID
	if _, ok := f.dedup[key]; ok {
		return false, nil
	}
	f.dedup[key] = struct{}{}
	return true, nil
}

func (f *fakeStore) TokenIssue(_ context.Context, scene Scene, tokenHex, fpHex string, _ time.Duration) (bool, error) {
	key := scene.String() + ":" + tokenHex
	if _, ok := f.tokens[key]; ok {
		return false, nil
	}
	f.tokens[key] = fpHex
	return true, nil
}

func (f *fakeStore) TokenConsume(_ context.Context, scene Scene, tokenHex string) (string, bool, error) {
	key := scene.String() + ":" + tokenHex
	v, ok := f.tokens[key]
	if !ok {
		return "", false, nil
	}
	delete(f.tokens, key)
	return v, true, nil
}

var (
	_ = zap.NewNop
	_ = redis.Nil
)

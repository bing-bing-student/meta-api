// Package share 实现 JSON 工具"分享创建"链路的风控守卫预检与一次性 token 签发/消费。
//
// 设计要点：
//  1. Go 侧只做"是否真人"的风控判定（guard.Engine），不接管真正的 JSON 存储；
//     存储仍由 Nuxt 端的 /api/share-json（文件 + 索引）负责。
//  2. 预检通过后下发一次性 share-token（hex 64 chars，TTL 120s），写入 Redis：
//     guard:share-create:token:{tokenHex} → fingerprintHex
//  3. 业务侧（Nuxt）凭 token 调 /user/share/consume，原子读取并删除 token，
//     拿到 fingerprint 即可继续走原配额/限流/写文件流程。
//
// 文件分布：
//
//	service.go —— Service 接口 + impl 主流程（Precheck / Consume）
package share

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"time"

	"go.uber.org/zap"

	"meta-api/common/guard"
)

// 一次性 token 配置
const (
	// tokenTTL 一次性 token 在 Redis 中的存活时间。
	tokenTTL = 60 * time.Second

	// tokenBytes 生成的随机字节数。32B = 64 hex chars，与 view-log 信封 nonce 等量级。
	tokenBytes = 32

	// tokenIssueRetry SETNX 碰撞时的重试次数。32B 随机碰撞概率极低，
	// 但保留小重试可防御 Redis 历史 key 残留场景。
	tokenIssueRetry = 3
)

// PrecheckOutcome 预检的 HTTP 层输出。
//
// HTTPStatus 200 表示通过并返回 Token；其它状态对应 guard.Outcome.Decision 的映射。
// 未通过时 Token 为空，Reason 用于审计/前端追踪（不暴露细节）。
type PrecheckOutcome struct {
	HTTPStatus int
	Code       int
	Message    string
	// Token 仅在通过时非空。十六进制字符串（64 chars）。
	Token string
	// ExpiresIn 秒。前端可基于此判断是否需要重新预检。
	ExpiresIn int
}

// ConsumeOutcome 消费 token 的 HTTP 层输出。
//
// 命中时 Fingerprint 非空（64 hex chars）；未命中（已用 / 已过期 / 不存在）返回 401。
type ConsumeOutcome struct {
	HTTPStatus  int
	Code        int
	Message     string
	Fingerprint string
}

// Service 分享创建场景的风控守卫编排。
//
// 与 viewlog.Service 的差异：
//   - 不直接执行业务（写 JSON），仅签发"通行证"；
//   - 所有业务字段（jsonData/password/expires/...）继续由 Nuxt 端校验。
type Service interface {
	// Precheck 处理 envelope 预检请求。
	//
	// targetID 是分享草稿的 sha256 截 16B hex（前端在 sign 时传入），仅用于绑定信封；
	// Engine 通过即可放行，与具体 JSON 内容无关。
	Precheck(ctx context.Context, req *guard.RiskRequest) (*PrecheckOutcome, error)

	// Consume 消费 token，返回 fingerprint。
	//
	// 仅供内网调用（Nuxt SSR → meta-api），不应暴露到公网。
	Consume(ctx context.Context, tokenHex string) (*ConsumeOutcome, error)
}

// shareService Service 的具体实现。
type shareService struct {
	logger *zap.Logger
	engine guard.Engine
	store  guard.Store
}

// NewService 构造 share 风控服务实例。engine / store 必填。
func NewService(logger *zap.Logger, engine guard.Engine, store guard.Store) Service {
	return &shareService{
		logger: logger,
		engine: engine,
		store:  store,
	}
}

// Precheck 主流程：
//  1. 调 guard.Engine.Evaluate（与 view-log 共用同一引擎）
//  2. Decision != Accept → 按映射返回 HTTP 状态
//  3. Decision == Accept → 生成 token + SETNX 写 Redis → 返回给前端
func (s *shareService) Precheck(ctx context.Context, req *guard.RiskRequest) (*PrecheckOutcome, error) {
	if req == nil {
		return nil, errors.New("share: nil request")
	}
	out, err := s.engine.Evaluate(ctx, req)
	if err != nil {
		s.logger.Error("share precheck engine error", zap.Error(err))
		return &PrecheckOutcome{
			HTTPStatus: http.StatusInternalServerError,
			Code:       5000,
			Message:    "internal error",
		}, nil
	}

	switch out.Decision {
	case guard.DecisionAccept:
		// 通过：签发 token
		token, err := s.issueToken(ctx, out.Fingerprint)
		if err != nil {
			s.logger.Error("share precheck issue token failed", zap.Error(err))
			return &PrecheckOutcome{
				HTTPStatus: http.StatusInternalServerError,
				Code:       5000,
				Message:    "internal error",
			}, nil
		}
		return &PrecheckOutcome{
			HTTPStatus: http.StatusOK,
			Code:       2000,
			Message:    "ok",
			Token:      token,
			ExpiresIn:  int(tokenTTL / time.Second),
		}, nil
	case guard.DecisionSilent:
		// 静默拒：返回 200 + 不带 token，前端表现为"看似成功但无 token"，
		// 走到 Nuxt 时因 token 校验失败而失败。这里不暴露差异。
		//
		// 备选方案直接 401，但这会让脚本通过响应码立刻探测出风控守卫，
		// 故沿用 viewlog 的"silent = 看不出差异"约定。
		return &PrecheckOutcome{
			HTTPStatus: http.StatusOK,
			Code:       2000,
			Message:    "ok",
		}, nil
	case guard.DecisionRateLimited:
		return &PrecheckOutcome{
			HTTPStatus: http.StatusTooManyRequests,
			Code:       4290,
			Message:    "rate limited",
		}, nil
	case guard.DecisionBadRequest:
		return &PrecheckOutcome{
			HTTPStatus: http.StatusBadRequest,
			Code:       4000,
			Message:    "invalid token",
		}, nil
	case guard.DecisionInternal:
		return &PrecheckOutcome{
			HTTPStatus: http.StatusInternalServerError,
			Code:       5000,
			Message:    "internal error",
		}, nil
	default:
		return &PrecheckOutcome{
			HTTPStatus: http.StatusBadRequest,
			Code:       4000,
			Message:    "invalid token",
		}, nil
	}
}

// Consume 主流程：原子 GETDEL → 命中返回 fingerprint，未命中 401。
func (s *shareService) Consume(ctx context.Context, tokenHex string) (*ConsumeOutcome, error) {
	if !isValidTokenHex(tokenHex) {
		return &ConsumeOutcome{
			HTTPStatus: http.StatusUnauthorized,
			Code:       4010,
			Message:    "invalid token",
		}, nil
	}

	fp, ok, err := s.store.TokenConsume(ctx, guard.SceneShareCreate, tokenHex)
	if err != nil {
		s.logger.Warn("share consume token redis error", zap.Error(err))
		return &ConsumeOutcome{
			HTTPStatus: http.StatusInternalServerError,
			Code:       5000,
			Message:    "internal error",
		}, nil
	}
	if !ok {
		// 未命中：已被消费 / 已过期 / 不存在。统一 401。
		return &ConsumeOutcome{
			HTTPStatus: http.StatusUnauthorized,
			Code:       4010,
			Message:    "invalid token",
		}, nil
	}
	if !isValidFingerprintHex(fp) {
		// 防御：理论上写入端已校验过；万一 Redis 数据异常，这里再兜一次。
		s.logger.Warn("share consume token bad fingerprint", zap.String("fp_len", lenStr(fp)))
		return &ConsumeOutcome{
			HTTPStatus: http.StatusUnauthorized,
			Code:       4010,
			Message:    "invalid token",
		}, nil
	}

	return &ConsumeOutcome{
		HTTPStatus:  http.StatusOK,
		Code:        2000,
		Message:     "ok",
		Fingerprint: fp,
	}, nil
}

// issueToken 生成 + 落盘一次性 token，最多重试 tokenIssueRetry 次。
func (s *shareService) issueToken(ctx context.Context, fpHex string) (string, error) {
	if !isValidFingerprintHex(fpHex) {
		return "", errors.New("share: bad fingerprint from engine")
	}
	for i := 0; i < tokenIssueRetry; i++ {
		token, err := randomTokenHex()
		if err != nil {
			return "", err
		}
		ok, err := s.store.TokenIssue(ctx, guard.SceneShareCreate, token, fpHex, tokenTTL)
		if err != nil {
			return "", err
		}
		if ok {
			return token, nil
		}
		// SETNX 碰撞：重试。
	}
	return "", errors.New("share: token issue retry exceeded")
}

// randomTokenHex 生成 tokenBytes 字节的 crypto-rand → hex string。
func randomTokenHex() (string, error) {
	buf := make([]byte, tokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// isValidTokenHex token 必须是 64 字符的小写 hex。
func isValidTokenHex(s string) bool {
	if len(s) != tokenBytes*2 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'f':
		default:
			return false
		}
	}
	return true
}

// isValidFingerprintHex fingerprint 必须是 64 字符的小写 hex
// （FieldFingerprintID 在信封里固定 32 字节 → hex 编码后 64 字符）。
func isValidFingerprintHex(s string) bool {
	if len(s) != 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'f':
		default:
			return false
		}
	}
	return true
}

// lenStr 仅用于日志（避免泄漏 fingerprint 内容到错误日志）。
func lenStr(s string) string {
	const digits = "0123456789"
	if s == "" {
		return "0"
	}
	n := len(s)
	if n < 10 {
		return string(digits[n])
	}
	// 简单两位足够（指纹长度上限 32）。
	tens := n / 10
	ones := n % 10
	return string([]byte{digits[tens], digits[ones]})
}

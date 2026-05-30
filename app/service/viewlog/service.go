// Package viewlog 实现文章浏览量打点接口的业务编排。
//
//  1. RSA 解密 token，校验字段绑定 / 时间窗口 / nonce 防重放
//  2. 四层风控漏斗：L1 黑名单 → L2 可信度评分 → L3 去重&频控 → L4 审计日志
//  3. 通过后执行计数：Redis ZINCRBY ArticleViewZSet + HINCRBY ArticleHash.viewNum
//  4. 不论是否 +1，统一返回 204（429 频控除外，方便前端重试）
//
// 文件分布：
//
//	service.go —— Service 接口 + impl 入口 + DI 构造
//	token.go   —— RSA 解密 + JSON 解析 + 字段校验 + nonce SETNX
//	risk.go    —— L1/L2/L3 风控判定
//	counter.go —— 计数（Redis HINCRBY + ZINCRBY）
//	audit.go   —— 同步 zap 审计日志
//	types.go   —— 请求 / 明文 / Outcome / 评分常量 / 拒因常量
package viewlog

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	articleModel "meta-api/app/model/article"
	"meta-api/pkg/keymanager"
)

// dedupTTL (fp, articleId) 去重窗口
const dedupTTL = time.Minute

// uaBlacklist UA 黑名单（小写子串），匹配时对入参 UA 做 strings.ToLower 后子串包含判定。
var uaBlacklist = []string{
	"bot", "spider", "crawl", "slurp",
	"googlebot", "baiduspider", "bingbot", "yandexbot", "duckduckbot",
	"headless", "phantomjs",
	"curl", "wget", "python-requests", "go-http-client",
}

// Service 浏览量打点服务接口。
type Service interface {
	// PostViewLog 处理一次打点请求，返回 HTTP 层应使用的 Outcome。
	// 错误返回非 nil 表示出现意外异常（应映射为 500），业务拒绝走 Outcome 字段。
	PostViewLog(ctx context.Context, req *PostViewLogRequest) (*Outcome, error)
}

// viewLogService 浏览量打点服务实现。
type viewLogService struct {
	logger       *zap.Logger
	redis        *redis.Client
	keyManager   *keymanager.Manager
	articleModel articleModel.Model
}

// NewService 构造打点服务实例。
func NewService(logger *zap.Logger, rdb *redis.Client,
	km *keymanager.Manager, am articleModel.Model) Service {

	return &viewLogService{
		logger:       logger,
		redis:        rdb,
		keyManager:   km,
		articleModel: am,
	}
}

// PostViewLog 整体编排：token 校验 → 风控 → 计数 → 审计日志。
//
// 任一阶段判定为拒：填充 Outcome 后立即写审计日志返回，不进入下一阶段。
// 通过所有阶段：执行计数，写一条 accepted 日志，返回 204。
func (s *viewLogService) PostViewLog(ctx context.Context, req *PostViewLogRequest) (*Outcome, error) {
	serverNowMs := nowMillis()

	// 1. Token 解密与校验
	payload, outcome := s.verifyToken(ctx, req)
	if outcome != nil {
		s.audit(req, payload, decisionRejected, outcome.Reason, scoreStart, serverNowMs)
		return outcome.Result, nil
	}

	// 2. 文章存在性校验（404 与 token invalid 区分开）
	if outcome := s.ensureArticleExists(ctx, payload.ArticleID); outcome != nil {
		s.audit(req, payload, decisionRejected, outcome.Reason, scoreStart, serverNowMs)
		return outcome.Result, nil
	}

	// 3. L1 硬规则黑名单
	if outcome := s.checkL1(ctx, req); outcome != nil {
		s.audit(req, payload, decisionRejected, outcome.Reason, scoreStart, serverNowMs)
		return outcome.Result, nil
	}

	// 4. L2 可信度评分（含 referer 直接拒）
	score, outcome := s.checkL2(req, payload, serverNowMs)
	if outcome != nil {
		s.audit(req, payload, decisionRejected, outcome.Reason, score, serverNowMs)
		return outcome.Result, nil
	}

	// 5. L3 去重 + 频控
	if outcome := s.checkL3(ctx, req, payload); outcome != nil {
		s.audit(req, payload, decisionRejected, outcome.Reason, score, serverNowMs)
		return outcome.Result, nil
	}

	// 6. 计数：失败仅打日志，不影响响应（避免攻击者通过响应差异探测）
	s.increment(ctx, payload.ArticleID)

	// 7. 审计日志
	s.audit(req, payload, decisionAccepted, "", score, serverNowMs)

	return &Outcome{HTTPStatus: 204}, nil
}

// rejectOutcome 内部辅助类型：携带 HTTP Outcome + 审计 reason。
type rejectOutcome struct {
	Result *Outcome
	Reason string
}

// nowMillis 当前服务端时间（毫秒）。提取出来便于测试时可替换。
var nowMillis = func() int64 {
	return timeNow().UnixMilli()
}

// secondsToDuration 秒整数 → time.Duration，避免在调用处反复乘 time.Second。
func secondsToDuration(s int) time.Duration {
	return time.Duration(s) * time.Second
}

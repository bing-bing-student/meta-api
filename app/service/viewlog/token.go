package viewlog

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"meta-api/pkg/keymanager"
)

// timeNow 当前时间的可替换钩子，便于单元测试控制时钟。
var timeNow = time.Now

// nonceCacheKey nonce 防重放在 Redis 中的命名空间。
func nonceCacheKey(nonce string) string {
	return "view-log:nonce:" + nonce
}

// base64 → RSA decrypt → JSON parse → articleId 绑定 → ts 窗口 → nonce SETNX。
//
// 返回 (payload, nil) 表示 token 合法可继续后续风控；
// 返回 (payload, *rejectOutcome) 表示已确定拒绝，payload 可能仅有部分字段供审计日志使用。
func (s *viewLogService) verifyToken(ctx context.Context, req *PostViewLogRequest) (*TokenPayload, *rejectOutcome) {
	emptyPayload := &TokenPayload{ArticleID: req.ArticleID}

	// 1. Base64 解码
	ciphertext, err := base64.StdEncoding.DecodeString(req.Token)
	if err != nil {
		return emptyPayload, badRequestReject(reasonTokenInvalid)
	}

	// 2. RSA 解密（current 失败回退 previous，由 keymanager 内部处理）
	plaintext, err := s.keyManager.Decrypt(ciphertext)
	if err != nil {
		// 区分 KeyManager 未就绪与解密失败：日志细节差异在 reason 之外靠 logger 标注
		if errors.Is(err, keymanager.ErrNotReady) {
			s.logger.Warn("view-log key manager not ready")
		}
		return emptyPayload, badRequestReject(reasonTokenInvalid)
	}

	// 3. JSON 解析
	var payload TokenPayload
	if err := json.Unmarshal(plaintext, &payload); err != nil {
		return emptyPayload, badRequestReject(reasonTokenInvalid)
	}

	// 4. articleId 绑定校验：URL 与 token 必须一致，防止"拿一篇的 token 刷另一篇"
	if payload.ArticleID == "" || payload.ArticleID != req.ArticleID {
		return &payload, badRequestReject(reasonTokenArticleMismatch)
	}

	// 5. 时间窗口校验：±2 分钟
	skew := timeNow().UnixMilli() - payload.TS
	if skew < 0 {
		skew = -skew
	}
	if skew > tsSkewToleranceMs {
		return &payload, badRequestReject(reasonTokenTSSkew)
	}

	// 6. nonce 防重放：与去重窗口对齐为 1 分钟
	if payload.Nonce == "" {
		return &payload, badRequestReject(reasonTokenInvalid)
	}
	ok, err := s.redis.SetNX(ctx, nonceCacheKey(payload.Nonce), 1, time.Minute).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		s.logger.Warn("view-log nonce setnx failed", zap.Error(err))
		// Redis 异常不能放过 token，按 invalid 处理（保守策略）
		return &payload, badRequestReject(reasonTokenInvalid)
	}
	if !ok {
		return &payload, badRequestReject(reasonTokenReplay)
	}

	return &payload, nil
}

// ensureArticleExists 校验 articleId 在 MySQL 中真实存在。
// 不存在 → 404；其它 DB 错误 → 500。
//
// 命中现有 GORM Model 接口 GetArticleDetailByID，避免新增方法。
func (s *viewLogService) ensureArticleExists(ctx context.Context, articleIDStr string) *rejectOutcome {
	id, err := parseArticleID(articleIDStr)
	if err != nil {
		return notFoundReject(reasonArticleNotFound)
	}
	_, err = s.articleModel.GetArticleDetailByID(ctx, id)
	if err != nil {
		// 与现有代码一致：MySQL 未命中既可能是 sql.ErrNoRows，也可能 GORM 返回字符串 "record not found"
		if isNotFoundErr(err) {
			return notFoundReject(reasonArticleNotFound)
		}
		s.logger.Error("view-log article exists check failed",
			zap.String("article_id", articleIDStr), zap.Error(err))
		return internalErrorReject()
	}
	return nil
}

// parseArticleID 字符串雪花 ID → uint64，与现有 idutil.ParseID 保持同样的 0 拒绝语义。
// 这里独立实现避免对 idutil 包产生反向依赖（idutil 不依赖 viewlog）。
func parseArticleID(s string) (uint64, error) {
	var id uint64
	if s == "" {
		return 0, fmt.Errorf("empty id")
	}
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("invalid id")
		}
		id = id*10 + uint64(ch-'0')
	}
	if id == 0 {
		return 0, fmt.Errorf("zero id")
	}
	return id, nil
}

// isNotFoundErr GORM v2 record not found 判定。
func isNotFoundErr(err error) bool {
	if err == nil {
		return false
	}
	// 与 service/article/user.go 中现有判定保持一致
	return strings.Contains(err.Error(), "record not found")
}

// badRequestReject 构造 400 拒绝。
func badRequestReject(reason string) *rejectOutcome {
	return &rejectOutcome{
		Result: &Outcome{HTTPStatus: 400, Code: codeBadRequest, Message: "invalid token"},
		Reason: reason,
	}
}

// notFoundReject 构造 404 拒绝。
func notFoundReject(reason string) *rejectOutcome {
	return &rejectOutcome{
		Result: &Outcome{HTTPStatus: 404, Code: codeNotFound, Message: "article not found"},
		Reason: reason,
	}
}

// rateLimitedReject 构造 429 拒绝。
func rateLimitedReject(reason string) *rejectOutcome {
	return &rejectOutcome{
		Result: &Outcome{HTTPStatus: 429, Code: codeRateLimited, Message: "rate limited"},
		Reason: reason,
	}
}

// silentReject 构造 204 静默拒绝（风控判定不 +1 但不暴露细节）。
func silentReject(reason string) *rejectOutcome {
	return &rejectOutcome{
		Result: &Outcome{HTTPStatus: 204},
		Reason: reason,
	}
}

// internalErrorReject 构造 500 拒绝。
func internalErrorReject() *rejectOutcome {
	return &rejectOutcome{
		Result: &Outcome{HTTPStatus: 500, Code: codeInternalError, Message: "internal error"},
		Reason: reasonInternalError,
	}
}

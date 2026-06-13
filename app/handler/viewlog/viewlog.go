package viewlog

import (
	"bytes"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"meta-api/app/service/viewlog"
	"meta-api/common/guard"
	"meta-api/common/types"
)

// envelopeMagic 与 crypto-wasm/src/envelope.rs 中常量保持一致。
// 用于在 Handler 入口快速识别 body 是否为 GUAR 信封。
var envelopeMagic = [4]byte{'G', 'U', 'A', 'R'}

// PostViewLog 处理 POST /user/article/view-log/:id。
//
// 仅接受 GUAR 信封请求；非 GUAR 信封一律 400。
//
// 职责：
//  1. 绑定 URL :id 与请求体
//  2. 从 Header 采集后端"自取"字段（IP / UA / Sec-Fetch-*）
//  3. 将聚合请求交给 guard.Engine 处理
//  4. 按 Outcome 映射 HTTP 状态：204 / 400 / 404 / 429 / 500
func (h *viewLogHandler) PostViewLog(c *gin.Context) {
	c.Header("Cache-Control", "no-store, private")

	articleID := c.Param("id")
	if articleID == "" {
		c.Status(http.StatusBadRequest)
		c.Abort()
		return
	}

	// 读 body（限制 16KB，与 guard.MaxBodyBytes 对齐）。
	body, err := readLimitedBody(c.Request, h.logger, guard.MaxBodyBytes)
	if err != nil {
		h.logger.Debug("view-log read body failed", zap.Error(err))
		c.Status(http.StatusBadRequest)
		c.Abort()
		return
	}

	if !isGuardEnvelope(body) {
		c.Status(http.StatusBadRequest)
		c.Abort()
		return
	}

	h.handleByGuard(c, articleID, body)
}

// readLimitedBody 读取请求体，最大 maxBytes。超过会被 io.LimitReader 截断（不返回 error）。
//
// 为简单起见使用一次性读取；body 限定 16KB 量级，内存压力可忽略。
//
// Body.Close 错误用闭包显式记录：HTTP server 端 Body.Close 失败极罕见
// （正常情况下框架已读完），但若发生（如客户端早断 + reverse proxy 异常），
// 仍记录为 Debug 级便于排查。
func readLimitedBody(r *http.Request, logger *zap.Logger, maxBytes int64) ([]byte, error) {
	if r.Body == nil {
		return nil, errors.New("nil body")
	}
	defer func() {
		if err := r.Body.Close(); err != nil {
			logger.Debug("view-log request body close failed", zap.Error(err))
		}
	}()
	return io.ReadAll(io.LimitReader(r.Body, maxBytes))
}

// isGuardEnvelope 判定 body 是否新信封：仅检查 4 字节 magic。
//
// 设计取舍：
//   - 不校验 version / scene 字段 —— 那些由 guard.Engine 自己处理并按 BadRequest 拒
//   - 仅靠 magic 区分新链路入口
func isGuardEnvelope(body []byte) bool {
	return len(body) >= 4 && bytes.Equal(body[:4], envelopeMagic[:])
}

// handleByGuard 新链路：guard.Engine.Evaluate → +1 → 返回。
func (h *viewLogHandler) handleByGuard(c *gin.Context, articleID string, body []byte) {
	r := c.Request
	req := &guard.RiskRequest{
		Scene:     guard.SceneViewLog,
		TargetID:  articleID,
		RawBody:   body,
		ClientIP:  c.ClientIP(),
		UserAgent: r.UserAgent(),
		Referer:   r.Referer(),
		SecFetch: guard.SecFetchHeaders{
			Mode:           r.Header.Get("Sec-Fetch-Mode"),
			Site:           r.Header.Get("Sec-Fetch-Site"),
			Dest:           r.Header.Get("Sec-Fetch-Dest"),
			AcceptLanguage: r.Header.Get("Accept-Language"),
		},
	}

	out, err := h.engine.Evaluate(c.Request.Context(), req)
	if err != nil {
		// engine 返回 error 仅在内部异常（参数 nil 等）发生，按 500 兜底
		h.logger.Error("guard evaluate unexpected error", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 5000, "message": "internal error"})
		return
	}

	switch out.Decision {
	case guard.DecisionAccept:
		if rej := h.service.EnsureArticleExists(c.Request.Context(), articleID); rej != nil {
			respondViewlogOutcome(c, rej)
			return
		}
		h.service.Increment(c.Request.Context(), articleID)
		c.Status(http.StatusNoContent)
	case guard.DecisionSilent:
		// 静默拒：返回 204 不暴露细节
		c.Status(http.StatusNoContent)
	case guard.DecisionRateLimited:
		c.JSON(http.StatusTooManyRequests, types.Response{Code: 4290, Message: "rate limited"})
	case guard.DecisionNotFound:
		c.JSON(http.StatusNotFound, types.Response{Code: 4040, Message: "article not found"})
	case guard.DecisionInternal:
		c.JSON(http.StatusInternalServerError, types.Response{Code: 5000, Message: "internal error"})
	case guard.DecisionBadRequest:
		fallthrough
	default:
		c.JSON(http.StatusBadRequest, types.Response{Code: 4000, Message: "invalid token"})
	}
}

// respondViewlogOutcome 把 viewlog.Outcome 转成 HTTP 响应。
//
// 仅用于新链路调用 service.EnsureArticleExists 后拿到的非空 Outcome（404 / 500）。
func respondViewlogOutcome(c *gin.Context, out *viewlog.Outcome) {
	switch out.HTTPStatus {
	case http.StatusNoContent:
		c.Status(http.StatusNoContent)
	default:
		c.JSON(out.HTTPStatus, types.Response{Code: out.Code, Message: out.Message})
	}
}

package share

import (
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"meta-api/common/guard"
	"meta-api/common/types"
)

// targetIDQueryKey 前端在 URL query 中传递 targetId（draftHash 截 16B hex），
// 与 view-log 把 articleID 放在 path 不同：share-create 没有"目标资源 ID"这种
// 自然路径维度，targetId 只用于 envelope 内字段绑定，放 query 即可。
const targetIDQueryKey = "target_id"

// targetIDMaxLen targetId 长度上限（hex 32 chars = 16B 截 sha256）。
//
// 不强制最小长度，避免约束未来前端改用其它 hash 长度；最大长度防御异常输入。
const targetIDMaxLen = 64

// tokenHeader 一次性 token 的传递头。
const tokenHeader = "X-Guard-Token"

// Precheck 处理 POST /user/share/precheck。
//
// 流程：
//  1. 必须为 octet-stream，body 限制 16KB；
//  2. 从 query 取 target_id（与 envelope 内字段绑定校验）；
//  3. 调 service.Precheck → guard.Engine.Evaluate；
//  4. 通过则返回 token；不通过按 service 给出的状态码返回。
func (h *shareHandler) Precheck(c *gin.Context) {
	c.Header("Cache-Control", "no-store, private")

	targetID := c.Query(targetIDQueryKey)
	if targetID == "" || len(targetID) > targetIDMaxLen {
		c.JSON(http.StatusBadRequest, types.Response{Code: 4000, Message: "invalid token"})
		return
	}

	body, err := readLimitedBody(c.Request.Body, h.logger, guard.MaxBodyBytes)
	if err != nil {
		h.logger.Debug("share precheck read body failed", zap.Error(err))
		c.JSON(http.StatusBadRequest, types.Response{Code: 4000, Message: "invalid token"})
		return
	}

	r := c.Request
	req := &guard.RiskRequest{
		Scene:     guard.SceneShareCreate,
		TargetID:  targetID,
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

	out, err := h.service.Precheck(c.Request.Context(), req)
	if err != nil {
		h.logger.Error("share precheck unexpected error", zap.Error(err))
		c.JSON(http.StatusInternalServerError, types.Response{Code: 5000, Message: "internal error"})
		return
	}

	if out.HTTPStatus == http.StatusOK {
		// 通过 / 静默拒：均返回 200，但只有 token 非空才能继续走业务。
		// 静默拒不暴露差异：响应结构与通过完全一致，仅缺 token 字段。
		c.JSON(http.StatusOK, gin.H{
			"code":       out.Code,
			"message":    out.Message,
			"token":      out.Token,
			"expires_in": out.ExpiresIn,
		})
		return
	}

	c.JSON(out.HTTPStatus, types.Response{Code: out.Code, Message: out.Message})
}

// Consume 处理 POST /user/share/consume。
//
// 流程：
//  1. 从 X-Guard-Token header 读取 token；
//  2. service.Consume 原子 GETDEL；
//  3. 命中返回 fingerprint，未命中 401。
//
// 注意：此端点仅供内网（Nuxt SSR → meta-api）调用。生产环境应在网关层
// 或 nginx 层做来源 IP 限制，避免 token 在公网被旁路消费。
func (h *shareHandler) Consume(c *gin.Context) {
	c.Header("Cache-Control", "no-store, private")

	token := c.GetHeader(tokenHeader)
	out, err := h.service.Consume(c.Request.Context(), token)
	if err != nil {
		h.logger.Error("share consume unexpected error", zap.Error(err))
		c.JSON(http.StatusInternalServerError, types.Response{Code: 5000, Message: "internal error"})
		return
	}

	if out.HTTPStatus == http.StatusOK {
		c.JSON(http.StatusOK, gin.H{
			"code":        out.Code,
			"message":     out.Message,
			"fingerprint": out.Fingerprint,
		})
		return
	}

	c.JSON(out.HTTPStatus, types.Response{Code: out.Code, Message: out.Message})
}

// readLimitedBody 读取请求体并限制最大字节数，超过 maxBytes 即截断（不报错）。
//
// 与 viewlog.readLimitedBody 行为对齐；这里独立实现避免跨 handler 包的紧耦合。
//
// Body.Close 错误用闭包显式记录为 Debug 级，便于在客户端早断等异常路径排查。
func readLimitedBody(rc io.ReadCloser, logger *zap.Logger, maxBytes int64) ([]byte, error) {
	if rc == nil {
		return nil, errors.New("nil body")
	}
	defer func() {
		if err := rc.Close(); err != nil {
			logger.Debug("share request body close failed", zap.Error(err))
		}
	}()
	return io.ReadAll(io.LimitReader(rc, maxBytes))
}

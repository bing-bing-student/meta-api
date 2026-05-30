package viewlog

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"meta-api/app/service/viewlog"
	"meta-api/common/types"
)

// PostViewLog 处理 POST /user/article/view-log/:id。
//
// 职责：
//  1. 绑定 URL :id 与请求体字段
//  2. 从 Header 采集后端"自取"字段（IP / UA / Sec-Fetch-*）
//  3. 将聚合请求交给 viewlog.Service
//  4. 按 service 返回的 Outcome 映射 HTTP 状态：204 / 400 / 404 / 429 / 500
func (h *viewLogHandler) PostViewLog(c *gin.Context) {
	c.Header("Cache-Control", "no-store, private")

	articleID := c.Param("id")
	if articleID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 4000, "message": "invalid token"})
		return
	}

	req := &viewlog.PostViewLogRequest{ArticleID: articleID}
	if err := c.ShouldBindJSON(req); err != nil {
		// token 缺失或 JSON 解析失败，统一按 invalid token 处理
		h.logger.Debug("view-log bind json failed", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"code": 4000, "message": "invalid token"})
		return
	}
	req.ArticleID = articleID // 防止前端在 body 里塞 articleID 覆盖 URL

	// 后端自行采集字段
	r := c.Request
	req.IP = c.ClientIP()
	req.UserAgent = r.UserAgent()
	req.AcceptLanguage = r.Header.Get("Accept-Language")
	req.SecFetchMode = r.Header.Get("Sec-Fetch-Mode")
	req.SecFetchSite = r.Header.Get("Sec-Fetch-Site")
	req.SecFetchDest = r.Header.Get("Sec-Fetch-Dest")

	outcome, err := h.service.PostViewLog(c.Request.Context(), req)
	if err != nil {
		// service 返回 error 视为意外异常，按 500 兜底；正常拒绝走 outcome
		h.logger.Error("view-log service unexpected error", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 5000, "message": "internal error"})
		return
	}

	switch outcome.HTTPStatus {
	case http.StatusNoContent:
		c.Status(http.StatusNoContent)
	default:
		c.JSON(outcome.HTTPStatus, types.Response{
			Code:    outcome.Code,
			Message: outcome.Message,
		})
	}
}

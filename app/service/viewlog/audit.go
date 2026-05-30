package viewlog

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// 日志级别约定：
//   - decision == accepted                              → INFO
//   - rejected 且 reason ∈ {L1_*, L3_DEDUP}             → INFO（预期拦截，量大不刷 WARN）
//   - rejected 且 reason ∈ {TOKEN_*, L3_RATE_*, L2_*}   → WARN（需关注的异常）
//   - 其它（INTERNAL_ERROR / ARTICLE_NOT_FOUND）         → WARN
//
// 选择直接同步 logger.Info / Warn 而不是 channel + 异步：
//  1. 本站 PV 量级（< 1w/天）不会让 zap 同步路径成为瓶颈
//  2. 同步写不会丢消息，便于后续 grep 回溯
//  3. 简化失败模式（少一条 channel 容量 + 丢弃计数器告警链路）
func (s *viewLogService) audit(req *PostViewLogRequest, payload *TokenPayload,
	decision, reason string, score int, serverNowMs int64) {

	if payload == nil {
		payload = &TokenPayload{ArticleID: req.ArticleID}
	}

	level := chooseAuditLevel(decision, reason)
	logFn := s.logger.Info
	if level == zapcore.WarnLevel {
		logFn = s.logger.Warn
	}

	logFn("view_log",
		zap.String("event", "view_log"),
		zap.String("article_id", payload.ArticleID),
		zap.String("fingerprint_id", payload.FingerprintID),
		zap.String("ip", req.IP),
		zap.String("user_agent", truncate(req.UserAgent, uaTruncateBytes)),
		zap.String("referer", truncate(req.Referer, refererTruncateBytes)),
		zap.String("tz", req.TZ),
		zap.String("screen", req.Screen),
		zap.String("lang", req.Lang),
		zap.String("perf_nav", req.PerfNav),
		zap.String("decision", decision),
		zap.String("reject_reason", reason),
		zap.Int("score", score),
		zap.Int64("client_ts", payload.TS),
		zap.Int64("server_ts", serverNowMs),
	)
}

func chooseAuditLevel(decision, reason string) zapcore.Level {
	if decision == decisionAccepted {
		return zapcore.InfoLevel
	}
	switch reason {
	case reasonL1UA, reasonL1Prerender, reasonL1Header, reasonL3Dedup:
		return zapcore.InfoLevel
	default:
		return zapcore.WarnLevel
	}
}

// truncate 把字符串按字节截断，保护日志不过长。
// 注意：截断点可能落在 UTF-8 多字节序列中间，这里保守按字节切；
// zap 的 JSON encoder 会把无效 UTF-8 转义，不会损坏日志结构。
func truncate(s string, maxBytes int) string {
	if maxBytes <= 0 || len(s) <= maxBytes {
		return s
	}
	return s[:maxBytes]
}

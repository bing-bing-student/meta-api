package guard

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// auditEntry 一次 Engine 评估的审计快照。
type auditEntry struct {
	Scene       Scene
	TargetID    string
	Fingerprint string
	IP          string
	UserAgent   string
	Decision    Decision
	Reason      string
	Score       int
	ClientTSMs  int64
	ServerTSMs  int64
}

// audit 输出一条审计日志。
//
// 日志级别约定：
//   - DecisionAccept                                    → INFO
//   - DecisionSilent 且 reason ∈ {L1_*, L3_DEDUP}        → INFO（预期拦截）
//   - DecisionRateLimited / DecisionBadRequest           → WARN
//   - DecisionInternal                                   → WARN
func (e *engine) audit(entry *auditEntry) {
	level := chooseAuditLevel(entry.Decision, entry.Reason)
	logFn := e.logger.Info
	if level == zapcore.WarnLevel {
		logFn = e.logger.Warn
	}

	logFn("guard_eval",
		zap.String("event", "guard_eval"),
		zap.String("scene", entry.Scene.String()),
		zap.String("target_id", entry.TargetID),
		zap.String("fingerprint_id", entry.Fingerprint),
		zap.String("ip", entry.IP),
		zap.String("user_agent", truncate(entry.UserAgent, uaTruncateBytes)),
		zap.Int("decision", int(entry.Decision)),
		zap.String("reject_reason", entry.Reason),
		zap.Int("score", entry.Score),
		zap.Int64("client_ts", entry.ClientTSMs),
		zap.Int64("server_ts", entry.ServerTSMs),
	)
}

// chooseAuditLevel 与 viewlog/audit.go 的 chooseAuditLevel 对齐。
func chooseAuditLevel(decision Decision, reason string) zapcore.Level {
	if decision == DecisionAccept {
		return zapcore.InfoLevel
	}
	if decision == DecisionSilent {
		switch reason {
		case ReasonL1UA, ReasonL1Prerender, ReasonL1Header, ReasonL3Dedup:
			return zapcore.InfoLevel
		}
		if hasPrefix(reason, ReasonViewLogCombined+":") {
			return zapcore.InfoLevel
		}
		if reason == ReasonViewLogDwellTooShort {
			return zapcore.InfoLevel
		}
	}
	return zapcore.WarnLevel
}

// hasPrefix 不引入 strings 包；reason 字符串拼接频繁，避免反向依赖。
func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

const uaTruncateBytes = 512

// truncate 按字节截断字符串。
func truncate(s string, maxBytes int) string {
	if maxBytes <= 0 || len(s) <= maxBytes {
		return s
	}
	return s[:maxBytes]
}

package moderation

import (
	"math"
	"strings"

	commentModel "meta-api/app/model/comment"
	appconfig "meta-api/config"
)

// baselineDecision 使用与最终决策引擎相同的证据标定和概率阈值，生成不含上下文纠偏的初判。
// 输入 signals 是检测器信号，cfg 是审核配置；返回值包含初判状态、分值、原因和决策代码。
func baselineDecision(signals []Signal, cfg appconfig.CommentModerationConfig) Result {
	reasons := make([]string, 0, len(signals))
	for _, signal := range signals {
		if signal.Reason != "" {
			reasons = append(reasons, signal.Reason)
		}
	}
	decision := fuseEvidence(evidenceFromSignals(signals, cfg), ContextAssessment{}, cfg.DecisionEngine)
	return Result{
		Status:   decision.Status,
		Score:    int(math.Round(decision.RiskProbability * 100)),
		Signals:  signals,
		Reasons:  reasons,
		Decision: evidenceBaselineDecisionCode(decision.Status),
	}
}

// evidenceBaselineDecisionCode 将审核状态 status 转换为规则初判阶段的决策代码并返回。
func evidenceBaselineDecisionCode(status string) string {
	switch status {
	case commentModel.StatusRejected:
		return "evidence_baseline_reject"
	case commentModel.StatusPending:
		return "evidence_baseline_review"
	default:
		return "evidence_baseline_allow"
	}
}

// disabledResult 构造审核功能关闭时的保守结果；无输入，返回待审核状态及 disabled 决策代码。
func disabledResult() Result {
	return Result{
		Status:   commentModel.StatusPending,
		Reasons:  []string{"moderation_disabled"},
		Decision: "disabled",
	}
}

// errorResult 根据 cfg 的失败策略将 err 转换为审核结果。
// 输入 err 是审核错误、cfg 提供默认失败状态；返回值为待审核或拒绝，并保留错误原因。
func errorResult(err error, cfg appconfig.CommentModerationConfig) Result {
	status := commentModel.StatusPending
	if strings.EqualFold(strings.TrimSpace(cfg.Decision.DefaultOnError), commentModel.StatusRejected) {
		status = commentModel.StatusRejected
	}
	return Result{
		Status:   status,
		Reasons:  []string{"moderation_error:" + err.Error()},
		Decision: "error",
	}
}

// evidenceStrengthScore 将信号来源、分类、规则和等级映射为百分制的启动期证据强度。
// 输入 cfg 提供标定参数；返回值范围为 0 至 100，不表示经过样本标定的真实概率。
func evidenceStrengthScore(source, category, ruleID, level string, cfg appconfig.CommentModerationConfig) int {
	confidence := bootstrapSignalConfidence(Signal{
		Source: source, Category: category, RuleID: ruleID, Level: level,
	}, cfg)
	return int(math.Round(confidence * 100))
}

// canonicalKeyPart 对 value 执行去空白和小写化，返回适合组成去重键的规范片段。
func canonicalKeyPart(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

// normalizeLevel 将 value 中的状态别名统一为审核等级常量；无法识别时返回空串。
func normalizeLevel(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case LevelBlock, "reject", "rejected":
		return LevelBlock
	case LevelReview, "pending":
		return LevelReview
	case LevelNotice:
		return LevelNotice
	case LevelAllow, "approved":
		return LevelAllow
	default:
		return ""
	}
}

// formatReason 按来源、分类、等级和证据拼接机器可读的审核原因。
// 输入中的空字段会被跳过；返回值使用冒号分隔有效字段。
func formatReason(source, category, level, evidence string) string {
	parts := []string{source}
	if category != "" {
		parts = append(parts, category)
	}
	if level != "" {
		parts = append(parts, level)
	}
	if evidence != "" {
		parts = append(parts, evidence)
	}
	return strings.Join(parts, ":")
}

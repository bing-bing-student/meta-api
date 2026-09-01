package moderation

import (
	"math"
	"strings"

	commentModel "meta-api/app/model/comment"
	appconfig "meta-api/config"
)

// baselineDecision builds the context-free first decision from the same evidence
// calibration and probability thresholds used by the final decision engine.
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

func disabledResult() Result {
	return Result{
		Status:   commentModel.StatusPending,
		Reasons:  []string{"moderation_disabled"},
		Decision: "disabled",
	}
}

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

func evidenceStrengthScore(source, category, ruleID, level string, cfg appconfig.CommentModerationConfig) int {
	confidence := bootstrapSignalConfidence(Signal{
		Source: source, Category: category, RuleID: ruleID, Level: level,
	}, cfg)
	return int(math.Round(confidence * 100))
}

func canonicalKeyPart(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

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

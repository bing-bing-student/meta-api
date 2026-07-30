package moderation

import (
	"strings"

	commentModel "meta-api/app/model/comment"
	appconfig "meta-api/config"
)

func decide(signals []Signal, cfg appconfig.CommentModerationConfig) Result {
	reasons := make([]string, 0, len(signals))
	score := 0
	hasReview := false
	for _, signal := range signals {
		if signal.Reason != "" {
			reasons = append(reasons, signal.Reason)
		}
		switch normalizeLevel(signal.Level) {
		case LevelBlock:
			return Result{
				Status:   commentModel.StatusRejected,
				Score:    scoreForLevel(LevelBlock, cfg),
				Signals:  signals,
				Reasons:  reasons,
				Decision: "block_signal",
			}
		case LevelReview:
			hasReview = true
			score += signal.Score
		default:
			score += signal.Score
		}
	}
	if hasReview {
		pendingScore := pendingScore(cfg)
		rejectScore := rejectScore(cfg)
		if score < pendingScore {
			score = pendingScore
		}
		if score >= rejectScore {
			score = rejectScore - 1
		}
		return Result{
			Status:   commentModel.StatusPending,
			Score:    score,
			Signals:  signals,
			Reasons:  reasons,
			Decision: "review_signal",
		}
	}
	return Result{
		Status:   commentModel.StatusApproved,
		Score:    score,
		Signals:  signals,
		Reasons:  reasons,
		Decision: "no_risk_signal",
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
		Score:    pendingScore(cfg),
		Reasons:  []string{"moderation_error:" + err.Error()},
		Decision: "error",
	}
}

func scoreForLevel(level string, cfg appconfig.CommentModerationConfig) int {
	switch normalizeLevel(level) {
	case LevelBlock:
		return rejectScore(cfg)
	case LevelReview:
		return pendingScore(cfg)
	default:
		return 0
	}
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

func pendingScore(cfg appconfig.CommentModerationConfig) int {
	if cfg.Decision.Score.Pending > 0 {
		return cfg.Decision.Score.Pending
	}
	return defaultPendingScore
}

func rejectScore(cfg appconfig.CommentModerationConfig) int {
	pending := pendingScore(cfg)
	if cfg.Decision.Score.Reject > pending {
		return cfg.Decision.Score.Reject
	}
	return defaultRejectScore
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

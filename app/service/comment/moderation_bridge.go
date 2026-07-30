package comment

import (
	"context"
	"time"

	commentModeration "meta-api/app/service/comment/moderation"
	appconfig "meta-api/config"
)

type commentModerationInput = commentModeration.Request
type commentModerationResult = commentModeration.Result
type commentModerationTextView = commentModeration.NormalizedComment
type commentModerationSignal = commentModeration.Signal
type commentModerationBehaviorState = commentModeration.BehaviorState
type commentModerationBehaviorKeys struct {
	user      string
	ip        string
	duplicate string
}

type commentModerationBehaviorRiskFunc func(context.Context, commentModerationInput,
	commentModerationTextView, appconfig.CommentModerationConfig) []commentModerationSignal

const (
	defaultCommentModerationPendingScore     = 40
	defaultCommentModerationRejectScore      = 80
	defaultCommentModerationSimilarityScore  = 40
	defaultCommentModerationDuplicatePending = 2
	commentModerationKeywordScore            = 40
	commentModerationRegexScore              = 40
	commentModerationUserRiskScore           = 40
	commentModerationIPRiskScore             = 40
	commentModerationDuplicateRiskScore      = 40
	commentModerationTextQualityRiskScore    = 40
	commentModerationEncodedURLRiskScore     = 40
	commentModerationPoliticalContextScore   = 40
	commentModerationSafetyContextScore      = 40
)

func (s *commentService) moderateComment(ctx context.Context, input commentModerationInput) commentModerationResult {
	return s.commentModerator().Moderate(ctx, input)
}

func (s *commentService) moderateCommentWithBehavior(ctx context.Context, input commentModerationInput,
	behaviorRisk commentModerationBehaviorRiskFunc,
) commentModerationResult {
	return s.commentModerator().ModerateWithBehavior(ctx, input,
		func(ctx context.Context, req commentModeration.Request, view commentModeration.NormalizedComment,
			cfg appconfig.CommentModerationConfig) []commentModeration.Signal {
			if behaviorRisk == nil {
				return nil
			}
			return behaviorRisk(ctx, req, view, cfg)
		},
	)
}

func (s *commentService) recordCommentModerationBehavior(ctx context.Context, input commentModerationInput) {
	s.commentModerator().RecordBehavior(ctx, input)
}

func (s *commentService) commentModerator() *commentModeration.Moderator {
	if s.moderator == nil {
		s.moderator = commentModeration.NewModerator(s.config, s.logger, s.redis)
	}
	return s.moderator
}

func newCommentModerationTextView(raw string) commentModerationTextView {
	return commentModeration.Normalize(raw)
}

func normalizeCommentModerationText(value string) string {
	return commentModeration.Normalize(value).Normalized
}

func compactCommentModerationText(value string) string {
	return commentModeration.Normalize(value).Compact
}

func buildCommentModerationBehaviorKeys(userID, articleID uint64, clientIP, normalized string) commentModerationBehaviorKeys {
	keys := commentModeration.BuildBehaviorKeys(userID, articleID, clientIP, normalized)
	return commentModerationBehaviorKeys{user: keys.User, ip: keys.IP, duplicate: keys.Duplicate}
}

func commentModerationBehaviorSignal(state commentModerationBehaviorState,
	cfg appconfig.CommentModerationConfig) commentModerationSignal {
	signals := commentModeration.BehaviorSignals(state, cfg)
	if len(signals) == 0 {
		return commentModerationSignal{}
	}
	return signals[0]
}

func fillCommentModerationDefaults(cfg *appconfig.CommentModerationConfig) {
	commentModeration.ApplyDefaults(cfg)
}

func commentModerationSecondsToDuration(seconds int64) time.Duration {
	if seconds <= 0 {
		return time.Second
	}
	return time.Duration(seconds) * time.Second
}

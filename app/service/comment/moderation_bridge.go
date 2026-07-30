package comment

import (
	"context"

	commentModeration "meta-api/app/service/comment/moderation"
	appconfig "meta-api/config"
)

type commentModerationInput = commentModeration.Request
type commentModerationResult = commentModeration.Result
type commentModerationTextView = commentModeration.NormalizedComment
type commentModerationSignal = commentModeration.Signal

type commentModerationBehaviorRiskFunc func(context.Context, commentModerationInput,
	commentModerationTextView, appconfig.CommentModerationConfig) []commentModerationSignal

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

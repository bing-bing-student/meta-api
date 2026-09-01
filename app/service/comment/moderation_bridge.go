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
	return s.applyModerationFeedbackPolicy(ctx, input, s.commentModerator().Moderate(ctx, input))
}

func (s *commentService) moderateCommentWithBehavior(ctx context.Context, input commentModerationInput,
	behaviorRisk commentModerationBehaviorRiskFunc,
) commentModerationResult {
	result := s.commentModerator().ModerateWithBehavior(ctx, input,
		func(ctx context.Context, req commentModeration.Request, view commentModeration.NormalizedComment,
			cfg appconfig.CommentModerationConfig) commentModeration.BehaviorEvaluation {
			if behaviorRisk == nil {
				return commentModeration.BehaviorEvaluation{Trace: commentModeration.BehaviorTrace{
					Status: "skipped", ReadOnly: true, UnavailableReason: "behavior_context_not_provided",
				}}
			}
			return commentModeration.BehaviorEvaluation{
				Signals: behaviorRisk(ctx, req, view, cfg),
				Trace: commentModeration.BehaviorTrace{
					Status: "executed", ReadOnly: true, ContextProvided: true,
				},
			}
		},
	)
	return s.applyModerationFeedbackPolicy(ctx, input, result)
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

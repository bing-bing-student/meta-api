package comment

import (
	"context"

	commentModeration "meta-api/app/service/comment/moderation"
	appconfig "meta-api/config"
)

// commentModerationInput 是评论服务使用的审核请求别名，避免业务层重复定义传输结构。
type commentModerationInput = commentModeration.Request

// commentModerationResult 是审核器完整结果在评论服务中的别名。
type commentModerationResult = commentModeration.Result

// commentModerationTextView 是审核器归一化文本视图在评论服务中的别名。
type commentModerationTextView = commentModeration.NormalizedComment

// commentModerationSignal 是审核原始信号在评论服务中的别名。
type commentModerationSignal = commentModeration.Signal

// commentModerationBehaviorRiskFunc 定义管理端模拟时可注入的只读行为风险计算函数。
// 输入依次为调用上下文、审核请求、归一化文本和审核配置；返回行为风险信号，不执行行为计数写入。
type commentModerationBehaviorRiskFunc func(context.Context, commentModerationInput,
	commentModerationTextView, appconfig.CommentModerationConfig) []commentModerationSignal

// moderateComment 执行完整评论审核并应用已形成共识的人工反馈策略。
// 输入 ctx 控制审核及反馈查询，input 是业务审核上下文；返回最终审核结果和决策轨迹。
func (s *commentService) moderateComment(ctx context.Context, input commentModerationInput) commentModerationResult {
	return s.applyModerationFeedbackPolicy(ctx, input, s.commentModerator().Moderate(ctx, input))
}

// moderateCommentWithBehavior 使用外部 behaviorRisk 提供的只读行为信号执行审核模拟，再应用人工反馈策略。
// 输入 ctx 和 input 是调用上下文及审核请求；返回模拟审核结果，behaviorRisk 为空时在轨迹中记录跳过原因。
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

// recordCommentModerationBehavior 在真实评论成功落库后记录 input 对应的用户、IP 和重复内容行为。
// 输入 ctx 控制 Redis 操作；方法无返回值，底层写入故障由审核器记录日志。
func (s *commentService) recordCommentModerationBehavior(ctx context.Context, input commentModerationInput) {
	s.commentModerator().RecordBehavior(ctx, input)
}

// commentModerator 返回服务持有的审核器，并在尚未初始化时使用当前依赖延迟创建。
func (s *commentService) commentModerator() *commentModeration.Moderator {
	if s.moderator == nil {
		s.moderator = commentModeration.NewModerator(s.config, s.logger, s.redis)
	}
	return s.moderator
}

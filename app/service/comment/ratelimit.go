package comment

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"meta-api/common/cachekey"
	"meta-api/common/ratelimit"
	appconfig "meta-api/config"
)

const (
	defaultCommentSubmitIPLimit           = 20
	defaultCommentSubmitIPWindow          = 10 * time.Minute
	defaultCommentSubmitUserLimit         = 10
	defaultCommentSubmitUserWindow        = 5 * time.Minute
	defaultCommentSubmitUserArticleLimit  = 5
	defaultCommentSubmitUserArticleWindow = 5 * time.Minute
	defaultCommentReportIPLimit           = 30
	defaultCommentReportIPWindow          = 24 * time.Hour
	defaultCommentReportUserLimit         = 10
	defaultCommentReportUserWindow        = 24 * time.Hour
	defaultCommentReportIPCommentLimit    = 2
	defaultCommentReportIPCommentWindow   = 24 * time.Hour
	unknownCommentRateLimitClientValue    = "unknown"
)

// commentSubmitLimitKeys 保存一次评论提交在 IP、用户和用户文章三个维度的 Redis 限流键。
type commentSubmitLimitKeys struct {
	ip          string
	user        string
	userArticle string
}

// commentReportLimitKeys 保存一次评论举报在 IP、用户和 IP 评论组合维度的 Redis 限流键。
type commentReportLimitKeys struct {
	ip        string
	user      string
	ipComment string
}

// checkCommentSubmitLimit 检查 userID、articleID 和 clientIP 对应的前台评论提交限流。
// 输入 ctx 控制 Redis 操作；未超限返回 nil，超限返回限流错误，存储故障转换为评论服务错误。
func (s *commentService) checkCommentSubmitLimit(ctx context.Context, userID, articleID uint64, clientIP string) error {
	cfg := s.commentSubmitRateLimitConfig()
	if cfg.Disabled {
		return nil
	}
	keys := buildCommentSubmitLimitKeys(userID, articleID, clientIP)
	err := s.limiter.Check(ctx,
		commentRateLimitRule(keys.ip, cfg.IP),
		commentRateLimitRule(keys.user, cfg.User),
		commentRateLimitRule(keys.userArticle, cfg.UserArticle),
	)
	return s.normalizeCommentRateLimitError(err)
}

// checkCommentReportLimit 检查 userID、commentID 和 clientIP 对应的前台评论举报限流。
// 输入 ctx 控制 Redis 操作；返回值规则与评论提交限流一致。
func (s *commentService) checkCommentReportLimit(ctx context.Context, userID, commentID uint64, clientIP string) error {
	cfg := s.commentReportRateLimitConfig()
	if cfg.Disabled {
		return nil
	}
	keys := buildCommentReportLimitKeys(userID, commentID, clientIP)
	err := s.limiter.Check(ctx,
		commentRateLimitRule(keys.ip, cfg.IP),
		commentRateLimitRule(keys.user, cfg.User),
		commentRateLimitRule(keys.ipComment, cfg.IPComment),
	)
	return s.normalizeCommentRateLimitError(err)
}

// buildCommentSubmitLimitKeys 根据 userID、articleID 和 clientIP 构造脱敏的评论提交 Redis 限流键并返回。
func buildCommentSubmitLimitKeys(userID, articleID uint64, clientIP string) commentSubmitLimitKeys {
	userHash := ratelimit.HashPart(strconv.FormatUint(userID, 10))
	articleHash := ratelimit.HashPart(strconv.FormatUint(articleID, 10))
	ipHash := ratelimit.HashPart(normalizeCommentRateLimitValue(clientIP))
	return commentSubmitLimitKeys{
		ip:          cachekey.CommentRateLimit("submit", "ip", ipHash).String(),
		user:        cachekey.CommentRateLimit("submit", "user", userHash).String(),
		userArticle: cachekey.CommentRateLimit("submit", "user-article", userHash, articleHash).String(),
	}
}

// buildCommentReportLimitKeys 根据 userID、commentID 和 clientIP 构造脱敏的举报 Redis 限流键并返回。
func buildCommentReportLimitKeys(userID, commentID uint64, clientIP string) commentReportLimitKeys {
	userHash := ratelimit.HashPart(strconv.FormatUint(userID, 10))
	commentHash := ratelimit.HashPart(strconv.FormatUint(commentID, 10))
	ipHash := ratelimit.HashPart(normalizeCommentRateLimitValue(clientIP))
	return commentReportLimitKeys{
		ip:        cachekey.CommentRateLimit("report", "ip", ipHash).String(),
		user:      cachekey.CommentRateLimit("report", "user", userHash).String(),
		ipComment: cachekey.CommentRateLimit("report", "ip-comment", ipHash, commentHash).String(),
	}
}

// normalizeCommentRateLimitError 将 err 中的存储层故障转换为评论业务错误，同时保留标准限流错误。
// 输入为空时返回 nil；不可识别故障会记录日志并返回暂不可用错误。
func (s *commentService) normalizeCommentRateLimitError(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := ratelimit.AsLimited(err); ok {
		return err
	}
	if s != nil && s.logger != nil {
		s.logger.Warn("comment rate-limit unavailable", zap.Error(err))
	}
	return errors.New("评论服务暂不可用，请稍后再试")
}

// commentSubmitRateLimitConfig 获取当前评论提交限流配置并填充默认值，返回可直接执行的独立配置。
func (s *commentService) commentSubmitRateLimitConfig() appconfig.CommentSubmitRateLimitConfig {
	cfg := appconfig.CommentSubmitRateLimitConfig{}
	if s != nil && s.config != nil {
		cfg = s.config.RateLimitSnapshot().CommentSubmit
	}
	fillCommentSubmitRateLimitDefaults(&cfg)
	return cfg
}

// commentReportRateLimitConfig 获取当前评论举报限流配置并填充默认值，返回可直接执行的独立配置。
func (s *commentService) commentReportRateLimitConfig() appconfig.CommentReportRateLimitConfig {
	cfg := appconfig.CommentReportRateLimitConfig{}
	if s != nil && s.config != nil {
		cfg = s.config.RateLimitSnapshot().CommentReport
	}
	fillCommentReportRateLimitDefaults(&cfg)
	return cfg
}

// fillCommentSubmitRateLimitDefaults 为 cfg 的 IP、用户和用户文章窗口原地填充默认值；无返回值。
func fillCommentSubmitRateLimitDefaults(cfg *appconfig.CommentSubmitRateLimitConfig) {
	fillCommentWindowConfig(&cfg.IP, defaultCommentSubmitIPLimit, defaultCommentSubmitIPWindow)
	fillCommentWindowConfig(&cfg.User, defaultCommentSubmitUserLimit, defaultCommentSubmitUserWindow)
	fillCommentWindowConfig(&cfg.UserArticle, defaultCommentSubmitUserArticleLimit, defaultCommentSubmitUserArticleWindow)
}

// fillCommentReportRateLimitDefaults 为 cfg 的 IP、用户和 IP 评论窗口原地填充默认值；无返回值。
func fillCommentReportRateLimitDefaults(cfg *appconfig.CommentReportRateLimitConfig) {
	fillCommentWindowConfig(&cfg.IP, defaultCommentReportIPLimit, defaultCommentReportIPWindow)
	fillCommentWindowConfig(&cfg.User, defaultCommentReportUserLimit, defaultCommentReportUserWindow)
	fillCommentWindowConfig(&cfg.IPComment, defaultCommentReportIPCommentLimit, defaultCommentReportIPCommentWindow)
}

// fillCommentWindowConfig 使用 defaultLimit 和 defaultWindow 原地修正 cfg 中非正数限制及窗口；无返回值。
func fillCommentWindowConfig(cfg *appconfig.RateLimitWindowConfig, defaultLimit int64, defaultWindow time.Duration) {
	if cfg.Limit <= 0 {
		cfg.Limit = defaultLimit
	}
	if cfg.WindowSeconds <= 0 {
		cfg.WindowSeconds = int64(defaultWindow / time.Second)
	}
}

// commentRateLimitRule 将 key 和窗口 cfg 转换为限流器可执行的 Rule 并返回。
func commentRateLimitRule(key string, cfg appconfig.RateLimitWindowConfig) ratelimit.Rule {
	return ratelimit.Rule{
		Key:    key,
		Limit:  cfg.Limit,
		Window: commentSecondsToDuration(cfg.WindowSeconds),
	}
}

// commentSecondsToDuration 将 seconds 转换为 time.Duration；非正数按一秒处理，返回始终为正的时长。
func commentSecondsToDuration(seconds int64) time.Duration {
	if seconds <= 0 {
		return time.Second
	}
	return time.Duration(seconds) * time.Second
}

// normalizeCommentRateLimitValue 对限流身份 value 去空白并转小写；空值返回稳定的 unknown 占位符。
func normalizeCommentRateLimitValue(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return unknownCommentRateLimitClientValue
	}
	return value
}

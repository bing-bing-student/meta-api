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
	unknownCommentRateLimitClientValue    = "unknown"
)

type commentSubmitLimitKeys struct {
	ip          string
	user        string
	userArticle string
}

// checkCommentSubmitLimit 检查前台评论提交限流。
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

// buildCommentSubmitLimitKeys 构造评论提交相关 Redis 限流 Key。
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

// normalizeCommentRateLimitError 将存储层错误转换为评论提交错误。
func (s *commentService) normalizeCommentRateLimitError(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := ratelimit.AsLimited(err); ok {
		return err
	}
	if s != nil && s.logger != nil {
		s.logger.Warn("comment submit rate-limit unavailable", zap.Error(err))
	}
	return errors.New("评论服务暂不可用，请稍后再试")
}

// commentSubmitRateLimitConfig 获取当前评论提交限流配置并填充默认值。
func (s *commentService) commentSubmitRateLimitConfig() appconfig.CommentSubmitRateLimitConfig {
	cfg := appconfig.CommentSubmitRateLimitConfig{}
	if s != nil && s.config != nil {
		cfg = s.config.RateLimitSnapshot().CommentSubmit
	}
	fillCommentSubmitRateLimitDefaults(&cfg)
	return cfg
}

// fillCommentSubmitRateLimitDefaults 填充评论提交限流默认配置。
func fillCommentSubmitRateLimitDefaults(cfg *appconfig.CommentSubmitRateLimitConfig) {
	fillCommentWindowConfig(&cfg.IP, defaultCommentSubmitIPLimit, defaultCommentSubmitIPWindow)
	fillCommentWindowConfig(&cfg.User, defaultCommentSubmitUserLimit, defaultCommentSubmitUserWindow)
	fillCommentWindowConfig(&cfg.UserArticle, defaultCommentSubmitUserArticleLimit, defaultCommentSubmitUserArticleWindow)
}

// fillCommentWindowConfig 填充单条窗口规则默认值。
func fillCommentWindowConfig(cfg *appconfig.RateLimitWindowConfig, defaultLimit int64, defaultWindow time.Duration) {
	if cfg.Limit <= 0 {
		cfg.Limit = defaultLimit
	}
	if cfg.WindowSeconds <= 0 {
		cfg.WindowSeconds = int64(defaultWindow / time.Second)
	}
}

// commentRateLimitRule 将配置项转换为限流规则。
func commentRateLimitRule(key string, cfg appconfig.RateLimitWindowConfig) ratelimit.Rule {
	return ratelimit.Rule{
		Key:    key,
		Limit:  cfg.Limit,
		Window: commentSecondsToDuration(cfg.WindowSeconds),
	}
}

// commentSecondsToDuration 将配置秒数转换为 Duration。
func commentSecondsToDuration(seconds int64) time.Duration {
	if seconds <= 0 {
		return time.Second
	}
	return time.Duration(seconds) * time.Second
}

// normalizeCommentRateLimitValue 标准化评论限流身份值。
func normalizeCommentRateLimitValue(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return unknownCommentRateLimitClientValue
	}
	return value
}

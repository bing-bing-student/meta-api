package moderation

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"meta-api/common/cachekey"
	"meta-api/common/ratelimit"
	appconfig "meta-api/config"
)

type BehaviorStore struct {
	redis  *redis.Client
	logger *zap.Logger
}

func NewBehaviorStore(redis *redis.Client, logger *zap.Logger) *BehaviorStore {
	return &BehaviorStore{redis: redis, logger: logger}
}

func (s *BehaviorStore) Signals(ctx context.Context, req Request, text NormalizedComment,
	cfg appconfig.CommentModerationConfig) []Signal {
	if s == nil || s.redis == nil {
		return nil
	}
	keys := BuildBehaviorKeys(req.UserID, req.ArticleID, req.ClientIP, text.Normalized)
	userCount, userErr := s.countEvents(ctx, keys.User, req.Now, durationSeconds(userRule(cfg).WindowSeconds))
	ipCount, ipErr := s.countEvents(ctx, keys.IP, req.Now, durationSeconds(ipRule(cfg).WindowSeconds))
	duplicateCount, duplicateErr := s.getCounter(ctx, keys.Duplicate)
	if userErr != nil || ipErr != nil || duplicateErr != nil {
		if s.logger != nil {
			s.logger.Warn("comment moderation behavior risk unavailable",
				zap.Error(errors.Join(userErr, ipErr, duplicateErr)))
		}
		return nil
	}
	return BehaviorSignals(BehaviorState{
		UserCount:      userCount,
		IPCount:        ipCount,
		DuplicateCount: duplicateCount,
	}, cfg)
}

func (s *BehaviorStore) Record(ctx context.Context, req Request, cfg appconfig.CommentModerationConfig) {
	if s == nil || s.redis == nil || req.CommentID == 0 || cfg.Disabled {
		return
	}
	normalized := normalizeText(req.Content)
	keys := BuildBehaviorKeys(req.UserID, req.ArticleID, req.ClientIP, normalized)
	nowScore := float64(req.Now.Unix())
	member := strconv.FormatUint(req.CommentID, 10)
	userWindow := durationSeconds(userRule(cfg).WindowSeconds)
	ipWindow := durationSeconds(ipRule(cfg).WindowSeconds)
	duplicateWindow := durationSeconds(duplicateRule(cfg).WindowSeconds)

	pipe := s.redis.Pipeline()
	pipe.ZRemRangeByScore(ctx, keys.User, "0", strconv.FormatInt(req.Now.Add(-userWindow).Unix()-1, 10))
	pipe.ZAdd(ctx, keys.User, redis.Z{Score: nowScore, Member: member})
	pipe.Expire(ctx, keys.User, userWindow+behaviorTTLExtra)
	pipe.ZRemRangeByScore(ctx, keys.IP, "0", strconv.FormatInt(req.Now.Add(-ipWindow).Unix()-1, 10))
	pipe.ZAdd(ctx, keys.IP, redis.Z{Score: nowScore, Member: member})
	pipe.Expire(ctx, keys.IP, ipWindow+behaviorTTLExtra)
	pipe.Incr(ctx, keys.Duplicate)
	pipe.Expire(ctx, keys.Duplicate, duplicateWindow)
	if _, err := pipe.Exec(ctx); err != nil && s.logger != nil {
		s.logger.Warn("failed to record comment moderation behavior", zap.Error(err))
	}
}

func BehaviorSignals(state BehaviorState, cfg appconfig.CommentModerationConfig) []Signal {
	signals := make([]Signal, 0, 3)
	user := userRule(cfg)
	if state.UserCount+1 >= user.ReviewThreshold {
		signals = append(signals, behaviorSignal("user_frequency", LevelReview, "user_frequency", cfg))
	}
	ip := ipRule(cfg)
	if state.IPCount+1 >= ip.ReviewThreshold {
		signals = append(signals, behaviorSignal("ip_frequency", LevelReview, "ip_frequency", cfg))
	}
	duplicate := duplicateRule(cfg)
	switch observed := state.DuplicateCount + 1; {
	case duplicate.BlockThreshold > 0 && observed >= duplicate.BlockThreshold:
		signals = append(signals, behaviorSignal("duplicate_content", LevelBlock, "duplicate_block", cfg))
	case duplicate.ReviewThreshold > 0 && observed >= duplicate.ReviewThreshold:
		signals = append(signals, behaviorSignal("duplicate_content", LevelReview, "duplicate_review", cfg))
	}
	return signals
}

func behaviorSignal(category, level, evidence string, cfg appconfig.CommentModerationConfig) Signal {
	return Signal{
		Source:   SourceBehavior,
		Category: category,
		Level:    level,
		Score:    scoreForLevel(level, cfg),
		Reason:   formatReason(SourceBehavior, category, level, evidence),
		Evidence: evidence,
		RuleID:   category,
	}
}

func (s *BehaviorStore) countEvents(ctx context.Context, key string, now time.Time, window time.Duration) (int64, error) {
	cutoff := now.Add(-window).Unix()
	return s.redis.ZCount(ctx, key, strconv.FormatInt(cutoff, 10), "+inf").Result()
}

func (s *BehaviorStore) getCounter(ctx context.Context, key string) (int64, error) {
	count, err := s.redis.Get(ctx, key).Int64()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	return count, err
}

func BuildBehaviorKeys(userID, articleID uint64, clientIP, normalized string) BehaviorKeys {
	userHash := ratelimit.HashPart(strconv.FormatUint(userID, 10))
	articleHash := ratelimit.HashPart(strconv.FormatUint(articleID, 10))
	ipHash := ratelimit.HashPart(normalizeRateLimitValue(clientIP))
	contentHash := ratelimit.HashPart(compactText(normalized))
	return BehaviorKeys{
		User:      cachekey.CommentModeration("behavior", "user", userHash).String(),
		IP:        cachekey.CommentModeration("behavior", "ip", ipHash).String(),
		Duplicate: cachekey.CommentModeration("behavior", "duplicate", userHash, articleHash, contentHash).String(),
	}
}

func normalizeRateLimitValue(value string) string {
	return normalizeText(value)
}

func userRule(cfg appconfig.CommentModerationConfig) appconfig.CommentModerationBehaviorThresholdConfig {
	rule := cfg.BehaviorRules.UserFrequency
	if rule.WindowSeconds <= 0 {
		rule.WindowSeconds = defaultUserWindowSeconds
	}
	if rule.ReviewThreshold <= 0 {
		rule.ReviewThreshold = defaultUserReviewThreshold
	}
	return rule
}

func ipRule(cfg appconfig.CommentModerationConfig) appconfig.CommentModerationBehaviorThresholdConfig {
	rule := cfg.BehaviorRules.IPFrequency
	if rule.WindowSeconds <= 0 {
		rule.WindowSeconds = defaultIPWindowSeconds
	}
	if rule.ReviewThreshold <= 0 {
		rule.ReviewThreshold = defaultIPReviewThreshold
	}
	return rule
}

func duplicateRule(cfg appconfig.CommentModerationConfig) appconfig.CommentModerationBehaviorThresholdConfig {
	rule := cfg.BehaviorRules.DuplicateContent
	if rule.WindowSeconds <= 0 {
		rule.WindowSeconds = defaultDuplicateWindowSeconds
	}
	if rule.ReviewThreshold <= 0 {
		rule.ReviewThreshold = defaultDuplicateReviewThreshold
	}
	if rule.BlockThreshold <= rule.ReviewThreshold {
		rule.BlockThreshold = defaultDuplicateBlockThreshold
	}
	return rule
}

func durationSeconds(seconds int64) time.Duration {
	if seconds <= 0 {
		return time.Second
	}
	return time.Duration(seconds) * time.Second
}

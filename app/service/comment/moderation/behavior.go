package moderation

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
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
	var userCount, ipCount, duplicateCount int64
	var userErr, ipErr, duplicateErr error
	userEvaluated := req.UserID != 0
	ipEvaluated := strings.TrimSpace(req.ClientIP) != ""
	duplicateEvaluated := req.UserID != 0 && req.ArticleID != 0
	if userEvaluated {
		userCount, userErr = s.countEvents(ctx, keys.User, req.Now, durationSeconds(userRule(cfg).WindowSeconds))
	}
	if ipEvaluated {
		ipCount, ipErr = s.countEvents(ctx, keys.IP, req.Now, durationSeconds(ipRule(cfg).WindowSeconds))
	}
	if duplicateEvaluated {
		duplicateCount, duplicateErr = s.getCounter(ctx, keys.Duplicate)
	}
	nearDuplicate := nearDuplicateRule(cfg)
	var nearDuplicateCount int64
	var nearDuplicateErr error
	nearDuplicateEvaluated := duplicateEvaluated && !nearDuplicate.Disabled
	if nearDuplicateEvaluated {
		nearDuplicateCount, nearDuplicateErr = s.countNearDuplicates(
			ctx,
			keys.NearDuplicate,
			req.Now,
			text.Normalized,
			nearDuplicate,
		)
	}
	if userErr != nil || ipErr != nil || duplicateErr != nil || nearDuplicateErr != nil {
		if s.logger != nil {
			s.logger.Warn("comment moderation behavior risk unavailable",
				zap.Error(errors.Join(userErr, ipErr, duplicateErr, nearDuplicateErr)))
		}
		return nil
	}
	return BehaviorSignals(BehaviorState{
		UserCount:              userCount,
		IPCount:                ipCount,
		DuplicateCount:         duplicateCount,
		NearDuplicateCount:     nearDuplicateCount,
		UserEvaluated:          userEvaluated,
		IPEvaluated:            ipEvaluated,
		DuplicateEvaluated:     duplicateEvaluated,
		NearDuplicateEvaluated: nearDuplicateEvaluated,
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
	nearDuplicate := nearDuplicateRule(cfg)
	nearDuplicateWindow := durationSeconds(nearDuplicate.WindowSeconds)

	pipe := s.redis.Pipeline()
	pipe.ZRemRangeByScore(ctx, keys.User, "0", strconv.FormatInt(req.Now.Add(-userWindow).Unix()-1, 10))
	pipe.ZAdd(ctx, keys.User, redis.Z{Score: nowScore, Member: member})
	pipe.Expire(ctx, keys.User, userWindow+behaviorTTLExtra)
	pipe.ZRemRangeByScore(ctx, keys.IP, "0", strconv.FormatInt(req.Now.Add(-ipWindow).Unix()-1, 10))
	pipe.ZAdd(ctx, keys.IP, redis.Z{Score: nowScore, Member: member})
	pipe.Expire(ctx, keys.IP, ipWindow+behaviorTTLExtra)
	pipe.Incr(ctx, keys.Duplicate)
	pipe.Expire(ctx, keys.Duplicate, duplicateWindow)
	contentRunes := len([]rune(compactText(normalized)))
	if !nearDuplicate.Disabled && contentRunes >= nearDuplicate.MinContentRunes {
		member := fmt.Sprintf("%016x:%d:%d", simHash(normalized), contentRunes, req.CommentID)
		pipe.ZRemRangeByScore(ctx, keys.NearDuplicate, "0",
			strconv.FormatInt(req.Now.Add(-nearDuplicateWindow).Unix()-1, 10))
		pipe.ZAdd(ctx, keys.NearDuplicate, redis.Z{Score: nowScore, Member: member})
		pipe.Expire(ctx, keys.NearDuplicate, nearDuplicateWindow+behaviorTTLExtra)
	}
	if _, err := pipe.Exec(ctx); err != nil && s.logger != nil {
		s.logger.Warn("failed to record comment moderation behavior", zap.Error(err))
	}
}

func BehaviorSignals(state BehaviorState, cfg appconfig.CommentModerationConfig) []Signal {
	signals := make([]Signal, 0, 3)
	user := userRule(cfg)
	if state.UserEvaluated && state.UserCount+1 >= user.ReviewThreshold {
		signals = append(signals, behaviorSignal("user_frequency", LevelReview, "user_frequency", cfg))
	}
	ip := ipRule(cfg)
	if state.IPEvaluated && state.IPCount+1 >= ip.ReviewThreshold {
		signals = append(signals, behaviorSignal("ip_frequency", LevelReview, "ip_frequency", cfg))
	}
	duplicate := duplicateRule(cfg)
	switch observed := state.DuplicateCount + 1; {
	case !state.DuplicateEvaluated:
	case duplicate.BlockThreshold > 0 && observed >= duplicate.BlockThreshold:
		signals = append(signals, behaviorSignal("duplicate_content", LevelBlock, "duplicate_block", cfg))
	case duplicate.ReviewThreshold > 0 && observed >= duplicate.ReviewThreshold:
		signals = append(signals, behaviorSignal("duplicate_content", LevelReview, "duplicate_review", cfg))
	}
	nearDuplicate := nearDuplicateRule(cfg)
	if state.NearDuplicateEvaluated && !nearDuplicate.Disabled &&
		state.NearDuplicateCount+1 >= nearDuplicate.ReviewThreshold {
		signals = append(signals, behaviorSignal("near_duplicate", LevelReview, "simhash_review", cfg))
	}
	return signals
}

func behaviorSignal(category, level, evidence string, cfg appconfig.CommentModerationConfig) Signal {
	return Signal{
		Source:   SourceBehavior,
		Category: category,
		Level:    level,
		Score:    scoreForSignal(SourceBehavior, category, category, level, cfg),
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

func (s *BehaviorStore) countNearDuplicates(ctx context.Context, key string, now time.Time,
	content string, rule appconfig.CommentModerationNearDuplicateConfig) (int64, error) {
	contentRunes := len([]rune(compactText(content)))
	if contentRunes < rule.MinContentRunes {
		return 0, nil
	}
	values, err := s.redis.ZRevRangeByScore(ctx, key, &redis.ZRangeBy{
		Min:    strconv.FormatInt(now.Add(-durationSeconds(rule.WindowSeconds)).Unix(), 10),
		Max:    "+inf",
		Offset: 0,
		Count:  rule.MaxSamples,
	}).Result()
	if err != nil {
		return 0, err
	}

	return countSimilarFingerprints(values, simHash(content), contentRunes, rule), nil
}

func countSimilarFingerprints(values []string, fingerprint uint64, contentRunes int,
	rule appconfig.CommentModerationNearDuplicateConfig) int64 {
	var count int64
	for _, value := range values {
		parts := strings.Split(value, ":")
		if len(parts) != 3 {
			continue
		}
		previousHash, hashErr := strconv.ParseUint(parts[0], 16, 64)
		previousLength, lengthErr := strconv.Atoi(parts[1])
		if hashErr != nil || lengthErr != nil ||
			!withinLengthDifference(contentRunes, previousLength, rule.MaxLengthDifferencePercent) {
			continue
		}
		if simHashDistance(fingerprint, previousHash) <= rule.MaxHammingDistance {
			count++
		}
	}
	return count
}

func withinLengthDifference(left, right, maxPercent int) bool {
	if left <= 0 || right <= 0 {
		return false
	}
	difference := left - right
	if difference < 0 {
		difference = -difference
	}
	return difference*100 <= max(left, right)*maxPercent
}

func BuildBehaviorKeys(userID, articleID uint64, clientIP, normalized string) BehaviorKeys {
	userHash := ratelimit.HashPart(strconv.FormatUint(userID, 10))
	articleHash := ratelimit.HashPart(strconv.FormatUint(articleID, 10))
	ipHash := ratelimit.HashPart(normalizeRateLimitValue(clientIP))
	contentHash := ratelimit.HashPart(compactText(normalized))
	return BehaviorKeys{
		User:          cachekey.CommentModeration("behavior", "user", userHash).String(),
		IP:            cachekey.CommentModeration("behavior", "ip", ipHash).String(),
		Duplicate:     cachekey.CommentModeration("behavior", "duplicate", userHash, articleHash, contentHash).String(),
		NearDuplicate: cachekey.CommentModeration("behavior", "near_duplicate", userHash, articleHash).String(),
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

func nearDuplicateRule(cfg appconfig.CommentModerationConfig) appconfig.CommentModerationNearDuplicateConfig {
	rule := cfg.BehaviorRules.NearDuplicate
	if rule.WindowSeconds <= 0 {
		rule.WindowSeconds = defaultNearDuplicateWindow
	}
	if rule.ReviewThreshold <= 0 {
		rule.ReviewThreshold = defaultNearDuplicateThreshold
	}
	if rule.MaxHammingDistance <= 0 {
		rule.MaxHammingDistance = defaultNearDuplicateDistance
	}
	if rule.MinContentRunes <= 0 {
		rule.MinContentRunes = defaultNearDuplicateMinRunes
	}
	if rule.MaxSamples <= 0 {
		rule.MaxSamples = defaultNearDuplicateMaxSamples
	}
	if rule.MaxLengthDifferencePercent <= 0 {
		rule.MaxLengthDifferencePercent = defaultNearDuplicateLengthDiff
	}
	return rule
}

func durationSeconds(seconds int64) time.Duration {
	if seconds <= 0 {
		return time.Second
	}
	return time.Duration(seconds) * time.Second
}

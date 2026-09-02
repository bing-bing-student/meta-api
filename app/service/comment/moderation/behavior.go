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

// BehaviorStore 封装基于 Redis 的评论频率、重复和近重复行为读写。
type BehaviorStore struct {
	redis  *redis.Client
	logger *zap.Logger
}

// NewBehaviorStore 使用 redis（行为数据客户端）和 logger（日志器）创建存储，返回 BehaviorStore。
func NewBehaviorStore(redis *redis.Client, logger *zap.Logger) *BehaviorStore {
	return &BehaviorStore{redis: redis, logger: logger}
}

// Signals 由 s 使用 ctx 查询 req 和 text 的行为状态，并按 cfg 评估，返回触发的行为信号。
func (s *BehaviorStore) Signals(ctx context.Context, req Request, text NormalizedComment,
	cfg appconfig.CommentModerationConfig) []Signal {
	return s.Evaluate(ctx, req, text, cfg).Signals
}

// Evaluate 由 s 使用 ctx 对 req 和 text 执行只读行为查询，并根据 cfg 生成信号；
// 返回同时包含信号和指标轨迹的 BehaviorEvaluation。
func (s *BehaviorStore) Evaluate(ctx context.Context, req Request, text NormalizedComment,
	cfg appconfig.CommentModerationConfig) BehaviorEvaluation {
	contextProvided := req.UserID != 0 || req.ArticleID != 0 || strings.TrimSpace(req.ClientIP) != ""
	if s == nil || s.redis == nil {
		return BehaviorEvaluation{Trace: BehaviorTrace{
			Status: "unavailable", ReadOnly: true, ContextProvided: contextProvided,
			UnavailableReason: "behavior_store_unavailable",
		}}
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
		return BehaviorEvaluation{Trace: BehaviorTrace{
			Status: "failed", ReadOnly: true, ContextProvided: contextProvided,
			UnavailableReason: "behavior_query_failed",
		}}
	}
	state := BehaviorState{
		UserCount:              userCount,
		IPCount:                ipCount,
		DuplicateCount:         duplicateCount,
		NearDuplicateCount:     nearDuplicateCount,
		UserEvaluated:          userEvaluated,
		IPEvaluated:            ipEvaluated,
		DuplicateEvaluated:     duplicateEvaluated,
		NearDuplicateEvaluated: nearDuplicateEvaluated,
	}
	signals := BehaviorSignals(state, cfg)
	status := "executed"
	unavailableReason := ""
	if !state.UserEvaluated && !state.IPEvaluated && !state.DuplicateEvaluated && !state.NearDuplicateEvaluated {
		status = "skipped"
		unavailableReason = "behavior_context_insufficient"
	}
	return BehaviorEvaluation{
		Signals: signals,
		Trace: BehaviorTrace{
			Status:            status,
			ReadOnly:          true,
			ContextProvided:   contextProvided,
			UnavailableReason: unavailableReason,
			Metrics:           behaviorMetricTrace(state, signals, cfg),
		},
	}
}

// behaviorMetricTrace 将 state（行为计数）、signals（触发信号）和 cfg（阈值配置）组装为指标轨迹列表。
func behaviorMetricTrace(state BehaviorState, signals []Signal,
	cfg appconfig.CommentModerationConfig,
) []BehaviorMetricTrace {
	user := userRule(cfg)
	ip := ipRule(cfg)
	duplicate := duplicateRule(cfg)
	nearDuplicate := nearDuplicateRule(cfg)
	nearDuplicateSkippedReason := "user_or_article_not_provided"
	if nearDuplicate.Disabled {
		nearDuplicateSkippedReason = "near_duplicate_disabled"
	}
	metrics := []BehaviorMetricTrace{
		{
			Name: "user_frequency", Evaluated: state.UserEvaluated,
			ObservedCount: state.UserCount, ProspectiveCount: state.UserCount + 1,
			WindowSeconds: user.WindowSeconds, ReviewThreshold: user.ReviewThreshold,
			SkippedReason: behaviorMetricSkippedReason(state.UserEvaluated, "user_id_not_provided"),
		},
		{
			Name: "ip_frequency", Evaluated: state.IPEvaluated,
			ObservedCount: state.IPCount, ProspectiveCount: state.IPCount + 1,
			WindowSeconds: ip.WindowSeconds, ReviewThreshold: ip.ReviewThreshold,
			SkippedReason: behaviorMetricSkippedReason(state.IPEvaluated, "client_ip_not_provided"),
		},
		{
			Name: "duplicate_content", Evaluated: state.DuplicateEvaluated,
			ObservedCount: state.DuplicateCount, ProspectiveCount: state.DuplicateCount + 1,
			WindowSeconds: duplicate.WindowSeconds, ReviewThreshold: duplicate.ReviewThreshold,
			BlockThreshold: duplicate.BlockThreshold,
			SkippedReason: behaviorMetricSkippedReason(state.DuplicateEvaluated,
				"user_or_article_not_provided"),
		},
		{
			Name: "near_duplicate", Evaluated: state.NearDuplicateEvaluated,
			ObservedCount: state.NearDuplicateCount, ProspectiveCount: state.NearDuplicateCount + 1,
			WindowSeconds: nearDuplicate.WindowSeconds, ReviewThreshold: nearDuplicate.ReviewThreshold,
			SkippedReason: behaviorMetricSkippedReason(state.NearDuplicateEvaluated,
				nearDuplicateSkippedReason),
		},
	}
	for index := range metrics {
		for _, signal := range signals {
			if signal.Category == metrics[index].Name {
				metrics[index].TriggeredLevel = signal.Level
				break
			}
		}
	}
	return metrics
}

// behaviorMetricSkippedReason 根据 evaluated 判断指标是否已评估；已评估返回空字符串，否则返回 reason。
func behaviorMetricSkippedReason(evaluated bool, reason string) string {
	if evaluated {
		return ""
	}
	return reason
}

// Record 由 s 使用 ctx 将 req 的已落库评论按 cfg 写入各行为窗口；无返回值，写入失败仅记录日志。
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

// BehaviorSignals 将 state 中的发布前计数与 cfg 阈值比较，返回频率、重复或近重复风险信号。
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

// behaviorSignal 根据 category、level、evidence 和 cfg 构造一条标准行为信号，返回 Signal。
func behaviorSignal(category, level, evidence string, cfg appconfig.CommentModerationConfig) Signal {
	return Signal{
		Source:   SourceBehavior,
		Category: category,
		Level:    level,
		Score:    evidenceStrengthScore(SourceBehavior, category, category, level, cfg),
		Reason:   formatReason(SourceBehavior, category, level, evidence),
		Evidence: evidence,
		RuleID:   category,
	}
}

// countEvents 由 s 统计 key 在 now 之前 window 时长内的有序集事件，返回数量和 Redis 错误。
func (s *BehaviorStore) countEvents(ctx context.Context, key string, now time.Time, window time.Duration) (int64, error) {
	cutoff := now.Add(-window).Unix()
	return s.redis.ZCount(ctx, key, strconv.FormatInt(cutoff, 10), "+inf").Result()
}

// getCounter 由 s 读取 key 的整数计数；key 不存在时返回 0，其他情况返回计数和错误。
func (s *BehaviorStore) getCounter(ctx context.Context, key string) (int64, error) {
	count, err := s.redis.Get(ctx, key).Int64()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	return count, err
}

// countNearDuplicates 由 s 在 key 的时间窗口内查找与 content 相似的指纹，rule 给出距离和样本限制；
// 返回近重复数量和 Redis 错误。
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

// countSimilarFingerprints 在 values 中按 fingerprint、contentRunes 和 rule 过滤可比指纹，返回汉明距离达标的数量。
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

// withinLengthDifference 比较 left 和 right 两个文本长度，返回它们的差异百分比是否不超过 maxPercent。
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

// BuildBehaviorKeys 使用 userID、articleID、clientIP 和 normalized（归一化内容）构造脱敏 Redis Key，返回 BehaviorKeys。
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

// normalizeRateLimitValue 对 value（限流维度原值）执行文本归一化，返回可稳定哈希的字符串。
func normalizeRateLimitValue(value string) string {
	return normalizeText(value)
}

// userRule 从 cfg 解析用户频率规则并补齐缺省值，返回有效阈值配置。
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

// ipRule 从 cfg 解析 IP 频率规则并补齐缺省值，返回有效阈值配置。
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

// duplicateRule 从 cfg 解析精确重复规则并修正无效阈值，返回有效配置。
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

// nearDuplicateRule 从 cfg 解析 SimHash 近重复规则并补齐窗口、距离和样本缺省值，返回有效配置。
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

// durationSeconds 将 seconds 转换为 time.Duration；非正数输入按 1 秒处理，返回始终为正的时长。
func durationSeconds(seconds int64) time.Duration {
	if seconds <= 0 {
		return time.Second
	}
	return time.Duration(seconds) * time.Second
}

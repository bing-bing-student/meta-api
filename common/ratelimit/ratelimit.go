package ratelimit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"
)

const defaultLimitedMessage = "请求过于频繁，请稍后再试"

// Rule 描述一条滑动窗口准入规则。
type Rule struct {
	Key    string
	Limit  int64
	Window time.Duration
}

// CheckResult 是 Store 对单条规则的判定结果。
type CheckResult struct {
	Allowed    bool
	RetryAfter time.Duration
	Count      int64
}

// Store 抽象限流底层存储，生产环境使用 Redis，单测可注入内存实现。
type Store interface {
	SlidingWindow(ctx context.Context, key string, limit int64, window time.Duration, now time.Time) (*CheckResult, error)
	Incr(ctx context.Context, key string, ttl time.Duration) (int64, error)
	TTL(ctx context.Context, key string) (time.Duration, error)
	SetTTL(ctx context.Context, key string, ttl time.Duration) error
	Del(ctx context.Context, keys ...string) error
}

// LimitedError 表示请求被限流或处于退避锁定期。
type LimitedError struct {
	RetryAfter time.Duration
	Message    string
}

// Error 返回对外展示的限流错误文案。
func (e *LimitedError) Error() string {
	if e == nil || e.Message == "" {
		return defaultLimitedMessage
	}
	return e.Message
}

// RetryAfterSeconds 返回向上取整后的重试等待秒数。
func (e *LimitedError) RetryAfterSeconds() int {
	if e == nil || e.RetryAfter <= 0 {
		return 1
	}
	return int((e.RetryAfter + time.Second - 1) / time.Second)
}

// NewLimitedError 构造统一的限流错误。
func NewLimitedError(retryAfter time.Duration) *LimitedError {
	if retryAfter <= 0 {
		retryAfter = time.Second
	}
	return &LimitedError{
		RetryAfter: retryAfter,
		Message:    defaultLimitedMessage,
	}
}

// AsLimited 判断错误是否为限流错误。
func AsLimited(err error) (*LimitedError, bool) {
	var limited *LimitedError
	if errors.As(err, &limited) {
		return limited, true
	}
	return nil, false
}

// BackoffConfig 描述连续失败后的指数退避策略。
type BackoffConfig struct {
	Threshold  int64
	CounterTTL time.Duration
	LevelTTL   time.Duration
	Durations  []time.Duration
}

// Limiter 组合滑动窗口准入和失败退避能力。
type Limiter struct {
	store Store
	now   func() time.Time
}

// NewLimiter 创建限流器实例。
func NewLimiter(store Store) *Limiter {
	return &Limiter{
		store: store,
		now:   time.Now,
	}
}

// SetNow 注入时间函数，主要用于单元测试。
func (l *Limiter) SetNow(now func() time.Time) {
	if now != nil {
		l.now = now
	}
}

// Check 按顺序检查多条滑动窗口规则。
func (l *Limiter) Check(ctx context.Context, rules ...Rule) error {
	if l == nil || l.store == nil {
		return nil
	}
	now := l.now()
	for _, rule := range rules {
		if rule.Key == "" || rule.Limit <= 0 || rule.Window <= 0 {
			continue
		}
		result, err := l.store.SlidingWindow(ctx, rule.Key, rule.Limit, rule.Window, now)
		if err != nil {
			return err
		}
		if !result.Allowed {
			return NewLimitedError(result.RetryAfter)
		}
	}
	return nil
}

// CheckLock 检查退避锁定 Key 是否仍在有效期内。
func (l *Limiter) CheckLock(ctx context.Context, key string) error {
	if l == nil || l.store == nil || key == "" {
		return nil
	}
	ttl, err := l.store.TTL(ctx, key)
	if err != nil {
		return err
	}
	if ttl > 0 {
		return NewLimitedError(ttl)
	}
	return nil
}

// RecordFailure 记录一次失败并在达到阈值后设置退避锁。
func (l *Limiter) RecordFailure(ctx context.Context, failKey, lockKey, levelKey string, cfg BackoffConfig) error {
	if l == nil || l.store == nil {
		return nil
	}
	if failKey == "" || lockKey == "" || levelKey == "" || cfg.Threshold <= 0 {
		return nil
	}
	count, err := l.store.Incr(ctx, failKey, cfg.CounterTTL)
	if err != nil {
		return err
	}
	if count < cfg.Threshold {
		return nil
	}

	level, err := l.store.Incr(ctx, levelKey, cfg.LevelTTL)
	if err != nil {
		return err
	}
	lockFor := cfg.durationForLevel(level)
	if err = l.store.SetTTL(ctx, lockKey, lockFor); err != nil {
		return err
	}
	if err = l.store.Del(ctx, failKey); err != nil {
		return err
	}
	return NewLimitedError(lockFor)
}

// Clear 删除限流相关 Key。
func (l *Limiter) Clear(ctx context.Context, keys ...string) error {
	if l == nil || l.store == nil || len(keys) == 0 {
		return nil
	}
	return l.store.Del(ctx, keys...)
}

// Increment 增加带 TTL 的计数器。
func (l *Limiter) Increment(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	if l == nil || l.store == nil || key == "" {
		return 0, nil
	}
	return l.store.Incr(ctx, key, ttl)
}

// durationForLevel 根据锁定等级获取对应退避时长。
func (cfg BackoffConfig) durationForLevel(level int64) time.Duration {
	if len(cfg.Durations) == 0 {
		return time.Minute
	}
	if level <= 0 {
		return cfg.Durations[0]
	}
	index := int(level - 1)
	if index >= len(cfg.Durations) {
		index = len(cfg.Durations) - 1
	}
	return cfg.Durations[index]
}

// HashPart 生成适合写入 Redis Key 的哈希片段。
func HashPart(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

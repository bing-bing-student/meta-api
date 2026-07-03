package ratelimit

import (
	"context"
	"testing"
	"time"
)

func TestLimiterSlidingWindow(t *testing.T) {
	now := time.Unix(100, 0)
	store := newMemoryStore(func() time.Time { return now })
	limiter := NewLimiter(store)
	limiter.SetNow(func() time.Time { return now })
	ctx := context.Background()

	rule := Rule{Key: "login:ip:1", Limit: 2, Window: 10 * time.Second}
	if err := limiter.Check(ctx, rule); err != nil {
		t.Fatalf("first request should pass: %v", err)
	}
	if err := limiter.Check(ctx, rule); err != nil {
		t.Fatalf("second request should pass: %v", err)
	}
	err := limiter.Check(ctx, rule)
	limited, ok := AsLimited(err)
	if !ok {
		t.Fatalf("third request should be limited, got %v", err)
	}
	if limited.RetryAfterSeconds() != 10 {
		t.Fatalf("retryAfter = %d, want 10", limited.RetryAfterSeconds())
	}

	now = now.Add(11 * time.Second)
	if err = limiter.Check(ctx, rule); err != nil {
		t.Fatalf("request should pass after window elapsed: %v", err)
	}
}

func TestLimiterRecordFailureBackoffEscalates(t *testing.T) {
	now := time.Unix(100, 0)
	store := newMemoryStore(func() time.Time { return now })
	limiter := NewLimiter(store)
	limiter.SetNow(func() time.Time { return now })
	ctx := context.Background()

	cfg := BackoffConfig{
		Threshold:  2,
		CounterTTL: time.Minute,
		LevelTTL:   time.Hour,
		Durations:  []time.Duration{2 * time.Minute, 5 * time.Minute},
	}
	if err := limiter.RecordFailure(ctx, "fail:user", "lock:user", "level:user", cfg); err != nil {
		t.Fatalf("first failure should not lock: %v", err)
	}
	err := limiter.RecordFailure(ctx, "fail:user", "lock:user", "level:user", cfg)
	limited, ok := AsLimited(err)
	if !ok {
		t.Fatalf("second failure should lock, got %v", err)
	}
	if limited.RetryAfter != 2*time.Minute {
		t.Fatalf("first lock duration = %s, want 2m", limited.RetryAfter)
	}
	if err = limiter.CheckLock(ctx, "lock:user"); err == nil {
		t.Fatal("lock should be active")
	}

	now = now.Add(3 * time.Minute)
	if err = limiter.CheckLock(ctx, "lock:user"); err != nil {
		t.Fatalf("lock should expire after time passes: %v", err)
	}
	if err = limiter.RecordFailure(ctx, "fail:user", "lock:user", "level:user", cfg); err != nil {
		t.Fatalf("first failure after lock expiry should not lock: %v", err)
	}
	err = limiter.RecordFailure(ctx, "fail:user", "lock:user", "level:user", cfg)
	limited, ok = AsLimited(err)
	if !ok {
		t.Fatalf("second round should lock, got %v", err)
	}
	if limited.RetryAfter != 5*time.Minute {
		t.Fatalf("second lock duration = %s, want 5m", limited.RetryAfter)
	}
}

type memoryStore struct {
	now      func() time.Time
	windows  map[string][]time.Time
	counters map[string]int64
	expires  map[string]time.Time
}

func newMemoryStore(now func() time.Time) *memoryStore {
	return &memoryStore{
		now:      now,
		windows:  make(map[string][]time.Time),
		counters: make(map[string]int64),
		expires:  make(map[string]time.Time),
	}
}

func (s *memoryStore) SlidingWindow(_ context.Context, key string, limit int64, window time.Duration, now time.Time) (*CheckResult, error) {
	cutoff := now.Add(-window)
	events := s.windows[key]
	kept := events[:0]
	for _, event := range events {
		if event.After(cutoff) {
			kept = append(kept, event)
		}
	}
	s.windows[key] = kept
	if int64(len(kept)) >= limit {
		retryAfter := kept[0].Add(window).Sub(now)
		if retryAfter < time.Second {
			retryAfter = time.Second
		}
		return &CheckResult{Allowed: false, RetryAfter: retryAfter, Count: int64(len(kept))}, nil
	}
	s.windows[key] = append(kept, now)
	return &CheckResult{Allowed: true, Count: int64(len(kept) + 1)}, nil
}

func (s *memoryStore) Incr(_ context.Context, key string, _ time.Duration) (int64, error) {
	s.counters[key]++
	return s.counters[key], nil
}

func (s *memoryStore) TTL(_ context.Context, key string) (time.Duration, error) {
	expireAt, ok := s.expires[key]
	if !ok {
		return -2 * time.Second, nil
	}
	ttl := expireAt.Sub(s.now())
	if ttl <= 0 {
		delete(s.expires, key)
		return -2 * time.Second, nil
	}
	return ttl, nil
}

func (s *memoryStore) SetTTL(_ context.Context, key string, ttl time.Duration) error {
	s.expires[key] = s.now().Add(ttl)
	return nil
}

func (s *memoryStore) Del(_ context.Context, keys ...string) error {
	for _, key := range keys {
		delete(s.windows, key)
		delete(s.counters, key)
		delete(s.expires, key)
	}
	return nil
}

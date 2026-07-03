package ratelimit

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

var slidingWindowScript = redis.NewScript(`
local key = KEYS[1]
local now = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])
local ttl = tonumber(ARGV[4])
local member = ARGV[5]

redis.call("ZREMRANGEBYSCORE", key, 0, now - window)
local count = redis.call("ZCARD", key)
if count >= limit then
	local oldest = redis.call("ZRANGE", key, 0, 0, "WITHSCORES")
	local retry_ms = 1000
	if oldest[2] then
		retry_ms = tonumber(oldest[2]) + window - now
	end
	if retry_ms < 1000 then
		retry_ms = 1000
	end
	return {0, retry_ms, count}
end

redis.call("ZADD", key, now, member)
redis.call("PEXPIRE", key, ttl)
return {1, 0, count + 1}
`)

// RedisStore 是基于 Redis 的限流存储实现。
type RedisStore struct {
	rdb *redis.Client
}

// NewRedisStore 创建 Redis 限流存储。
func NewRedisStore(rdb *redis.Client) *RedisStore {
	return &RedisStore{rdb: rdb}
}

// NewRedisLimiter 创建基于 Redis 的限流器。
func NewRedisLimiter(rdb *redis.Client) *Limiter {
	return NewLimiter(NewRedisStore(rdb))
}

// SlidingWindow 执行 Redis ZSET 滑动窗口准入判断。
func (s *RedisStore) SlidingWindow(ctx context.Context, key string, limit int64, window time.Duration, now time.Time) (*CheckResult, error) {
	if s == nil || s.rdb == nil {
		return &CheckResult{Allowed: true}, nil
	}
	windowMs := window.Milliseconds()
	if windowMs <= 0 {
		return &CheckResult{Allowed: true}, nil
	}
	member := strconv.FormatInt(now.UnixNano(), 10) + ":" + uuid.NewString()
	raw, err := slidingWindowScript.Run(ctx, s.rdb, []string{key},
		now.UnixMilli(),
		windowMs,
		limit,
		windowMs,
		member,
	).Slice()
	if err != nil {
		return nil, err
	}
	if len(raw) != 3 {
		return nil, fmt.Errorf("ratelimit: invalid redis script result length %d", len(raw))
	}
	allowed, err := scriptInt(raw[0])
	if err != nil {
		return nil, err
	}
	retryMs, err := scriptInt(raw[1])
	if err != nil {
		return nil, err
	}
	count, err := scriptInt(raw[2])
	if err != nil {
		return nil, err
	}
	return &CheckResult{
		Allowed:    allowed == 1,
		RetryAfter: time.Duration(retryMs) * time.Millisecond,
		Count:      count,
	}, nil
}

// Incr 增加计数器并在首次写入时设置 TTL。
func (s *RedisStore) Incr(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	count, err := s.rdb.Incr(ctx, key).Result()
	if err != nil {
		return 0, err
	}
	if count == 1 && ttl > 0 {
		if err = s.rdb.Expire(ctx, key, ttl).Err(); err != nil {
			return 0, err
		}
	}
	return count, nil
}

// TTL 返回 Key 的剩余有效期。
func (s *RedisStore) TTL(ctx context.Context, key string) (time.Duration, error) {
	return s.rdb.TTL(ctx, key).Result()
}

// SetTTL 写入锁定 Key 并设置有效期。
func (s *RedisStore) SetTTL(ctx context.Context, key string, ttl time.Duration) error {
	return s.rdb.Set(ctx, key, 1, ttl).Err()
}

// Del 删除一个或多个限流 Key。
func (s *RedisStore) Del(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	return s.rdb.Del(ctx, keys...).Err()
}

// scriptInt 将 Redis Lua 脚本返回值转换为整数。
func scriptInt(value any) (int64, error) {
	switch v := value.(type) {
	case int64:
		return v, nil
	case int:
		return int64(v), nil
	case string:
		return strconv.ParseInt(v, 10, 64)
	case []byte:
		return strconv.ParseInt(string(v), 10, 64)
	default:
		return 0, fmt.Errorf("ratelimit: unexpected redis script value %T", value)
	}
}

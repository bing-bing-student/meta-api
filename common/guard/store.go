package guard

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"meta-api/common/cachekey"
)

// Store 抽象 Redis 上的 nonce / 频控 / 主去重 / token 一次性签发与消费操作。
//
// 抽出 interface 是为了：
//  1. engine 单元测试可注入 fake Store，不强依赖真 Redis。
//  2. 未来若做"按场景独立 Redis 实例"，只需替换实现，engine 不需要改。
type Store interface {
	// NonceTrySet 在指定场景命名空间内 SETNX 一条 nonce。
	// 已存在返回 (false, nil)；新插入返回 (true, nil)；Redis 抖动返回 (false, err)。
	NonceTrySet(ctx context.Context, scene Scene, nonce []byte, ttl time.Duration) (bool, error)

	// IncrCheckRate INCR + 仅在首次时设置 EXPIRE，返回当前计数是否已超阈值。
	// Redis 异常时返回 (false, err)，由调用方决定是放行还是按"未超限"处理。
	IncrCheckRate(ctx context.Context, key string, ttl time.Duration, threshold int64) (bool, error)

	// DedupTrySet (fp, target) 主去重。命中返回 (false, nil)；首次返回 (true, nil)。
	DedupTrySet(ctx context.Context, scene Scene, fpHex, targetID string, ttl time.Duration) (bool, error)

	// TokenIssue 在指定场景命名空间下写入一次性 token，value 为 fingerprintHex。
	// SETNX：若 token 已存在返回 (false, nil)；写入成功返回 (true, nil)。
	// 用于 share-create 预检通过后下发"通行证"，业务侧凭 token 调用真正的存储接口。
	TokenIssue(ctx context.Context, scene Scene, tokenHex, fpHex string, ttl time.Duration) (bool, error)

	// TokenConsume 原子读取并删除一次性 token，命中返回 (fingerprintHex, true, nil)。
	// 未命中（不存在/已被消费/已过期）返回 ("", false, nil)。
	TokenConsume(ctx context.Context, scene Scene, tokenHex string) (string, bool, error)
}

// redisStore 默认 Store 实现。
type redisStore struct {
	rdb    *redis.Client
	logger *zap.Logger
}

// NewRedisStore 构造一个基于 go-redis 的 Store 实例。
func NewRedisStore(rdb *redis.Client, logger *zap.Logger) Store {
	return &redisStore{rdb: rdb, logger: logger}
}

// 命名空间约定与 cachekey 包风格保持一致：guard:{scene}:{kind}[:...]。
//
// 没把这些 key 移到 cachekey 包是因为 guard 包未来会独立演进
// （按场景配频控阈值），key 的生命周期与 cachekey 包内的业务 key 不耦合。
const guardKeyPrefix = "guard"

func nonceKey(scene Scene, nonce []byte) string {
	return guardKeyPrefix + ":" + scene.String() + ":nonce:" + bytesToHex(nonce)
}

func dedupKey(scene Scene, fpHex, targetID string) string {
	return guardKeyPrefix + ":" + scene.String() + ":dedup:" + fpHex + ":" + targetID
}

func tokenKey(scene Scene, tokenHex string) string {
	return guardKeyPrefix + ":" + scene.String() + ":token:" + tokenHex
}

// 频控 key 工厂（暂内联使用）。这里特意不暴露 helper 给外部，
// 所有频控阈值组合都集中在 engine.checkRate 中。
//
// 为避免 lint warning（unused），用 _ 关联到 cachekey.Key 的引用习惯。
var _ = cachekey.Key("")

func (s *redisStore) NonceTrySet(ctx context.Context, scene Scene, nonce []byte, ttl time.Duration) (bool, error) {
	if len(nonce) == 0 {
		return false, errors.New("guard: nonce empty")
	}
	ok, err := s.rdb.SetNX(ctx, nonceKey(scene, nonce), 1, ttl).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return false, err
	}
	return ok, nil
}

func (s *redisStore) IncrCheckRate(ctx context.Context, key string, ttl time.Duration, threshold int64) (bool, error) {
	cnt, err := s.rdb.Incr(ctx, key).Result()
	if err != nil {
		return false, err
	}
	if cnt == 1 {
		// 仅在首次设置 TTL，避免每次 INCR 续期形成"无限窗口"。
		if err := s.rdb.Expire(ctx, key, ttl).Err(); err != nil {
			s.logger.Warn("guard rate expire failed", zap.String("key", key), zap.Error(err))
		}
	}
	return cnt > threshold, nil
}

func (s *redisStore) DedupTrySet(ctx context.Context, scene Scene, fpHex, targetID string, ttl time.Duration) (bool, error) {
	ok, err := s.rdb.SetNX(ctx, dedupKey(scene, fpHex, targetID), 1, ttl).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return false, err
	}
	return ok, nil
}

// TokenIssue 一次性 token 签发：SETNX 防止极小概率的 token 碰撞覆盖既有记录。
//
// tokenHex 由调用方生成（典型 32B random → 64 hex chars），不在此函数生成是为了
// 让单元测试可以注入确定性 token。fpHex 必须是 64 hex chars（FieldFingerprintID 32 字节 → hex 编码 64 字符）。
func (s *redisStore) TokenIssue(ctx context.Context, scene Scene, tokenHex, fpHex string, ttl time.Duration) (bool, error) {
	if tokenHex == "" || fpHex == "" {
		return false, errors.New("guard: token/fp empty")
	}
	ok, err := s.rdb.SetNX(ctx, tokenKey(scene, tokenHex), fpHex, ttl).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return false, err
	}
	return ok, nil
}

// TokenConsume 原子读取并删除一次性 token。
//
// 使用 GETDEL（Redis 6.2+）保证读取与删除的原子性，避免并发场景下两路同时
// 通过验证。Redis 老版本可降级为 GET + DEL（非原子，存在极小并发缝隙）。
func (s *redisStore) TokenConsume(ctx context.Context, scene Scene, tokenHex string) (string, bool, error) {
	if tokenHex == "" {
		return "", false, nil
	}
	val, err := s.rdb.GetDel(ctx, tokenKey(scene, tokenHex)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", false, nil
		}
		return "", false, err
	}
	if val == "" {
		return "", false, nil
	}
	return val, true, nil
}

// bytesToHex 内联实现避免 import encoding/hex 时与其它文件命名冲突。
func bytesToHex(b []byte) string {
	const digits = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, v := range b {
		out[i*2] = digits[v>>4]
		out[i*2+1] = digits[v&0x0f]
	}
	return string(out)
}

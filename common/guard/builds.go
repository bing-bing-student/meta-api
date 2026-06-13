package guard

import (
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

// BuildHashRegistry 维护当前接受的 build_hash 集合，支持灰度多版本共存。
//
// 设计要点：
//  1. 多个版本可同时被接受（灰度期间老版本 wasm 客户端仍在跑）。
//  2. 单条记录可设置自动过期，配合"新版本上线 7 天后下线老版本"的运维策略。
//  3. 任意接口对并发安全。
//
// 当前未提供持久化与 admin 接口；初期可以在进程启动时通过 RegisterFromHex
// 灌入静态列表，后续版本通过配置中心或 admin api 热更新。
type BuildHashRegistry struct {
	mu      sync.RWMutex
	entries map[[fieldBuildHashLen]byte]time.Time // value = 过期时间；零值表示永不过期
	now     func() time.Time                      // 测试钩子
}

// NewBuildHashRegistry 构造空白白名单。
func NewBuildHashRegistry() *BuildHashRegistry {
	return &BuildHashRegistry{
		entries: make(map[[fieldBuildHashLen]byte]time.Time),
		now:     time.Now,
	}
}

// RegisterFromHex 注册一个 8 字节 build_hash（hex 字符串，长度 16）。
//
// expireAt 零值表示永不过期；非零值时将自动在 next gc 周期清理。
func (r *BuildHashRegistry) RegisterFromHex(buildHashHex string, expireAt time.Time) error {
	bh, err := decodeBuildHashHex(buildHashHex)
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.entries[bh] = expireAt
	r.mu.Unlock()
	return nil
}

// Unregister 主动下线一个 build_hash（运维用）。
func (r *BuildHashRegistry) Unregister(buildHashHex string) error {
	bh, err := decodeBuildHashHex(buildHashHex)
	if err != nil {
		return err
	}
	r.mu.Lock()
	delete(r.entries, bh)
	r.mu.Unlock()
	return nil
}

// Snapshot 返回当前有效（未过期）的所有 build_hash 列表，每条独立拷贝。
//
// engine 在 Evaluate 中调用，把候选列表传给 verifyHMAC。
func (r *BuildHashRegistry) Snapshot() [][]byte {
	r.mu.RLock()
	defer r.mu.RUnlock()
	now := r.now()
	out := make([][]byte, 0, len(r.entries))
	for k, exp := range r.entries {
		if !exp.IsZero() && exp.Before(now) {
			continue
		}
		copyBh := make([]byte, fieldBuildHashLen)
		copy(copyBh, k[:])
		out = append(out, copyBh)
	}
	return out
}

// Empty 返回当前是否没有任何有效 build_hash。
//
// 上线前期 registry 可能为空，此时 engine 应该按"放行 hmac 校验"处理（开发态）；
// 调用方决定开发态行为，不在 registry 内部假设。
func (r *BuildHashRegistry) Empty() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.entries) == 0
}

// decodeBuildHashHex 把 16 字符 hex 解析为固定 8 字节。
func decodeBuildHashHex(s string) ([fieldBuildHashLen]byte, error) {
	var out [fieldBuildHashLen]byte
	if len(s) != fieldBuildHashLen*2 {
		return out, errors.New("guard: build_hash hex length must be 16")
	}
	raw, err := hex.DecodeString(s)
	if err != nil {
		return out, err
	}
	copy(out[:], raw)
	return out, nil
}

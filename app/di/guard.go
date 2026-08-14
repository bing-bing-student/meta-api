package di

import (
	"fmt"
	"time"

	"go.uber.org/zap"

	"meta-api/common/guard"
	"meta-api/common/guard/keymanager"
	"meta-api/config"
)

// newGuardEngine 构造风控守卫引擎。
//
// 缺省 BuildHashes 为空 + SkipHMACWhenEmpty=true 即可平滑灰度（仍校验 RSA/AES/TLV）；
// 上线全量后通过 config.guard.build_hashes 注入 guard_hmac_build_hash.txt 的值，
// 并把 skip_hmac_when_empty 切回 false。
func newGuardEngine(cfg *config.Config, logger *zap.Logger, store guard.Store, km *keymanager.Manager) (guard.Engine, error) {
	gc := cfg.GuardConfig
	registry := guard.NewBuildHashRegistry()
	skipHMAC := true
	if gc != nil {
		if err := registerBuildHashes(registry, gc.BuildHashes); err != nil {
			return nil, fmt.Errorf("guard build_hashes invalid: %w", err)
		}
		skipHMAC = gc.SkipHMACWhenEmpty
	}
	return guard.NewEngine(guard.EngineConfig{
		KeyManager:        km,
		Store:             store,
		Logger:            logger,
		BuildHashes:       registry,
		SkipHMACWhenEmpty: skipHMAC,
	})
}

// registerBuildHashes 把配置中的 hex 字符串数组依次注册到 BuildHashRegistry。
//
// 任一条目格式不合法（非 16 字符 hex）即返回错误，让进程启动失败而不是默默放过。
// expireAt 留空表示永不过期；上线后可考虑配合配置中心动态下发 + 过期时间做老版本自动下线。
func registerBuildHashes(registry *guard.BuildHashRegistry, hashes []string) error {
	for _, h := range hashes {
		if h == "" {
			continue
		}
		if err := registry.RegisterFromHex(h, time.Time{}); err != nil {
			return fmt.Errorf("register build hash %q: %w", h, err)
		}
	}
	return nil
}

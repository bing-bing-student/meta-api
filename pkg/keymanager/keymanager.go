package keymanager

import (
	"crypto/rsa"
	"crypto/sha256"
	"errors"
	"sync"

	"go.uber.org/zap"
)

// ErrNotReady 表示 KeyManager 未加载到任何可用私钥（本地缺文件 / 启动失败）。
// 调用方收到此错误时应按 token 解密失败处理（返回 400）。
var ErrNotReady = errors.New("keyManager: no private key loaded")

// Manager 持有当前与上一轮 RSA 私钥，并在后台监听 keys 目录变更触发热更新。
//
// 实例由 DI 容器单例化构造。所有方法对并发安全。
// 当任一关键参数缺失或 fsnotify 初始化失败时，Manager 仍可构造，
// 但 DecryptOAEP 会一直返回 ErrNotReady，便于本地开发不依赖密钥文件。
type Manager struct {
	mu       sync.RWMutex
	current  *rsa.PrivateKey
	previous *rsa.PrivateKey

	keyDir   string // 监听目录，e.g. ./keys 或 /root/blog-website/keys
	logger   *zap.Logger
	debounce sync.Mutex // 串行化 reload 调用，避免并发 parse
}

// DecryptOAEP 用 current 私钥尝试 RSA-OAEP(SHA-256) 解密，失败时回退到 previous（如果存在）。
//
// 与前端 Rust `Oaep::new::<Sha256>()` 协议对齐：
//   - hash 算法：SHA-256（OAEP 内部用作 MGF1 与 label hash）
//   - label：固定为 nil（前端 wasm 端也未传 label）
//   - random：传 nil；OAEP 解密路径不需要随机数（仅加密路径用），nil 比传 rand.Reader 更清晰
//
// 入参为 RSA 密文原始字节（调用方负责 base64 / hex 解码）。
// 返回值为解密后的明文字节，调用方继续 Unmarshal / 解 TLV。
func (m *Manager) DecryptOAEP(ciphertext []byte) ([]byte, error) {
	m.mu.RLock()
	cur, prev := m.current, m.previous
	m.mu.RUnlock()

	if cur == nil {
		return nil, ErrNotReady
	}

	// 注意：每次 RSA-OAEP 调用都需要全新的 hash 实例，因为 stdlib 内部会消费它。
	plaintext, err := rsa.DecryptOAEP(sha256.New(), nil, cur, ciphertext, nil)
	if err == nil {
		return plaintext, nil
	}
	if prev != nil {
		if pt, err2 := rsa.DecryptOAEP(sha256.New(), nil, prev, ciphertext, nil); err2 == nil {
			return pt, nil
		}
	}
	return nil, err
}

// Package keymanager 负责管理用于浏览量打点接口的 RSA 私钥，并支持 fsnotify 热更新。
//
// 设计原则：
//   - 私钥仅在进程启动时与文件变更时加载，解密路径只走内存，避免每次请求 IO + parse；
//   - 同时持有 current + previous 两套私钥，覆盖宿主机 cron 轮换瞬间的兼容窗口；
//   - 永不向上抛错：fsnotify watcher 异常仅打 Warn 日志，不影响业务请求；
//   - 永不影响本地开发：当 keys 目录或 current 文件缺失时退化为 noop，所有 Decrypt 失败返回 ErrNotReady。
//
// 与生产部署对齐：
//
//	生成脚本 /root/blog-website/keys/generate_keys.sh 行为是
//	"先 mv 旧 private_key.pem → private_key.pem.prev，再 openssl genpkey 写入新 private_key.pem"。
//	因为 mv 会改变 inode，所以 watcher 监听的是 keys 目录而不是具体文件。
//	进程内 reload 不读 .prev：旧的 current 引用直接降级为 previous，新文件 parse 后成为 current。
//
// 文件分布：
//
//	keymanager.go —— Package doc + Manager 结构 + Decrypt 业务入口 + 错误定义
//	manager.go    —— New 构造 + 文件加载 + fsnotify 监听 goroutine + debounce 重新加载
package keymanager

import (
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"sync"

	"go.uber.org/zap"
)

// ErrNotReady 表示 KeyManager 未加载到任何可用私钥（本地缺文件 / 启动失败）。
// 调用方收到此错误时应按 token 解密失败处理（返回 400）。
var ErrNotReady = errors.New("keymanager: no private key loaded")

// Manager 持有当前与上一轮 RSA 私钥，并在后台监听 keys 目录变更触发热更新。
//
// 实例由 DI 容器单例化构造。所有方法对并发安全。
// 当任一关键参数缺失或 fsnotify 初始化失败时，Manager 仍可构造，
// 但 Decrypt 会一直返回 ErrNotReady，便于本地开发不依赖密钥文件。
type Manager struct {
	mu       sync.RWMutex
	current  *rsa.PrivateKey
	previous *rsa.PrivateKey

	keyDir   string // 监听目录，e.g. ./keys 或 /root/blog-website/keys
	logger   *zap.Logger
	debounce sync.Mutex // 串行化 reload 调用，避免并发 parse
}

// Decrypt 用 current 私钥尝试 PKCS#1 v1.5 解密，失败时回退到 previous（如果存在）。
//
// 入参为 RSA 密文原始字节（调用方负责 base64 解码）。
// 返回值为解密后的明文 JSON 字节，调用方继续 Unmarshal。
func (m *Manager) Decrypt(ciphertext []byte) ([]byte, error) {
	m.mu.RLock()
	cur, prev := m.current, m.previous
	m.mu.RUnlock()

	if cur == nil {
		return nil, ErrNotReady
	}

	plaintext, err := rsa.DecryptPKCS1v15(rand.Reader, cur, ciphertext)
	if err == nil {
		return plaintext, nil
	}
	if prev != nil {
		if pt, err2 := rsa.DecryptPKCS1v15(rand.Reader, prev, ciphertext); err2 == nil {
			return pt, nil
		}
	}
	return nil, err
}

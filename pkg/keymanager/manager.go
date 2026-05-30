package keymanager

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"go.uber.org/zap"
)

// 默认配置常量。
const (
	// envKeyDir 私钥目录的环境变量名。
	// 生产挂载点通常是 /root/blog-website/keys；本地默认 ./keys。
	envKeyDir = "KEY_DIR"

	// defaultKeyDir 未配置 KEY_DIR 时的默认相对路径。
	defaultKeyDir = "./keys"

	// privateKeyFile 当前生效的私钥文件名。生成脚本约定。
	privateKeyFile = "private_key.pem"

	// previousKeyFile 上一轮归档的私钥文件名（启动时若存在则加载为 previous）。
	previousKeyFile = "private_key.pem.prev"

	// debounceWindow fsnotify 在生成脚本"mv + create + write"的连续动作中
	// 通常会触发多个事件，用 200ms 防抖窗口收敛为一次 reload。
	debounceWindow = 200 * time.Millisecond
)

// New 构造一个 KeyManager。
//
// 启动流程：
//  1. 读取 KEY_DIR（默认 ./keys），尝试加载 current（私钥必需）与 previous（可选）
//  2. 启动 fsnotify watcher 监听 keyDir 目录，goroutine 收到 private_key.pem 的
//     Create / Write / Rename 事件后，经 debounce 触发 reload
//  3. 任何加载失败仅记录 Warn 日志，返回的 Manager 仍可调用，但 Decrypt 会返回 ErrNotReady
func New(logger *zap.Logger) *Manager {
	keyDir := os.Getenv(envKeyDir)
	if keyDir == "" {
		keyDir = defaultKeyDir
	}

	m := &Manager{
		keyDir: keyDir,
		logger: logger,
	}

	// 初始加载 current + previous
	if cur, err := loadPrivateKey(filepath.Join(keyDir, privateKeyFile)); err != nil {
		logger.Warn("keymanager disabled: load current key failed",
			zap.String("key_dir", keyDir), zap.Error(err))
	} else {
		m.current = cur
		logger.Info("keymanager loaded current key", zap.String("key_dir", keyDir))
	}

	if prev, err := loadPrivateKey(filepath.Join(keyDir, previousKeyFile)); err != nil {
		// .prev 不存在是正常的（首次部署），不打 warn
		if !errors.Is(err, os.ErrNotExist) {
			logger.Warn("keymanager load previous key failed",
				zap.String("key_dir", keyDir), zap.Error(err))
		}
	} else {
		m.previous = prev
		logger.Info("keymanager loaded previous key", zap.String("key_dir", keyDir))
	}

	// 启动 watcher（失败不影响 Decrypt，仅丧失热更新能力）
	if err := m.startWatcher(); err != nil {
		logger.Warn("keymanager watcher disabled", zap.String("key_dir", keyDir), zap.Error(err))
	}

	return m
}

// startWatcher 启动 fsnotify 监听 keyDir 目录，并在收到 private_key.pem 相关事件时触发 reload。
//
// 监听目录而非具体文件：生成脚本是 "mv 旧文件 + openssl 创建新文件" 的组合，
// 文件 inode 会变化，监听具体文件路径会在 mv 后丢失订阅。
func (m *Manager) startWatcher() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("new watcher: %w", err)
	}
	if err := watcher.Add(m.keyDir); err != nil {
		_ = watcher.Close()
		return fmt.Errorf("watch %s: %w", m.keyDir, err)
	}

	go m.watchLoop(watcher)
	m.logger.Info("keymanager watcher started", zap.String("key_dir", m.keyDir))
	return nil
}

// watchLoop 消费 fsnotify 事件，对 private_key.pem 的修改触发防抖重载。
func (m *Manager) watchLoop(watcher *fsnotify.Watcher) {
	defer watcher.Close()

	var (
		timer  *time.Timer
		timerC <-chan time.Time
	)

	for {
		select {
		case ev, ok := <-watcher.Events:
			if !ok {
				return
			}
			if !isPrivateKeyEvent(ev) {
				continue
			}
			// 重置 debounce 定时器：脚本 mv + openssl 至少触发 2 次事件
			if timer != nil {
				timer.Stop()
			}
			timer = time.NewTimer(debounceWindow)
			timerC = timer.C

		case <-timerC:
			timerC = nil
			m.reload()

		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			m.logger.Warn("keymanager watcher error", zap.Error(err))
		}
	}
}

// isPrivateKeyEvent 仅过滤 private_key.pem 上发生的有意义动作。
// 排除 .prev / public_key.pem 的事件（与解密路径无关），减少无谓 reload。
func isPrivateKeyEvent(ev fsnotify.Event) bool {
	if filepath.Base(ev.Name) != privateKeyFile {
		return false
	}
	return ev.Has(fsnotify.Create) || ev.Has(fsnotify.Write) || ev.Has(fsnotify.Rename)
}

// reload 重新加载 current 私钥；旧的 current 直接降级为 previous。
//
// 不读 .prev 文件：进程内已经持有的 current 比磁盘上的 .prev 更新更准（mv 之后的）。
// 用 debounce 互斥锁串行化，避免并发触发时 previous 被中间态覆盖。
func (m *Manager) reload() {
	m.debounce.Lock()
	defer m.debounce.Unlock()

	path := filepath.Join(m.keyDir, privateKeyFile)
	newCur, err := loadPrivateKey(path)
	if err != nil {
		m.logger.Warn("keymanager reload failed", zap.String("path", path), zap.Error(err))
		return
	}

	m.mu.Lock()
	oldCur := m.current
	m.current = newCur
	if oldCur != nil {
		m.previous = oldCur
	}
	m.mu.Unlock()

	m.logger.Info("keymanager rotated key",
		zap.String("path", path),
		zap.Bool("previous_set", oldCur != nil))
}

// loadPrivateKey 读取 PEM 文件并 parse 成 *rsa.PrivateKey。
// 兼容 PKCS#1（"RSA PRIVATE KEY"）与 PKCS#8（"PRIVATE KEY"）两种封装。
// openssl genpkey 生成的是 PKCS#8。
func loadPrivateKey(path string) (*rsa.PrivateKey, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, fmt.Errorf("invalid pem: %s", path)
	}

	switch {
	case strings.Contains(block.Type, "RSA PRIVATE KEY"):
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	case strings.Contains(block.Type, "PRIVATE KEY"):
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("not an rsa private key: %s", path)
		}
		return rsaKey, nil
	default:
		return nil, fmt.Errorf("unsupported pem type %q in %s", block.Type, path)
	}
}

package sitemap

import (
	"net/http"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"
)

// 默认配置常量。
const (
	// defaultTimeout 调用 portal-web sitemap 刷新接口的整体 HTTP 超时。
	defaultTimeout = 3 * time.Second

	// envEndpoint sitemap 刷新接口完整 URL 所在的环境变量名。
	envEndpoint = "SITEMAP_REVALIDATE_ENDPOINT"

	// envSecret sitemap 刷新接口共享密钥所在的环境变量名。
	envSecret = "SITEMAP_REVALIDATE_SECRET"

	// envSecretFile sitemap 刷新接口共享密钥文件路径所在的环境变量名。
	envSecretFile = "SITEMAP_REVALIDATE_SECRET_FILE"

	// legacyEnvEndpoint 兼容旧 Nuxt revalidate endpoint 环境变量。
	legacyEnvEndpoint = "NUXT_REVALIDATE_ENDPOINT"

	// legacyEnvSecret 兼容 portal-web 现有 revalidate secret 环境变量。
	legacyEnvSecret = "NUXT_REVALIDATE_SECRET"

	// legacyEnvSecretFile 兼容 portal-web 现有 revalidate secret file 环境变量。
	legacyEnvSecretFile = "NUXT_REVALIDATE_SECRET_FILE"
)

// Client 用来调用 portal-web /api/_revalidate 刷新 sitemap 内部缓存。
type Client struct {
	endpoint string
	secret   string
	timeout  time.Duration
	http     *http.Client
	logger   *zap.Logger
}

// New 构造 sitemap 刷新客户端。
func New(logger *zap.Logger) *Client {
	endpoint := firstNonEmpty(os.Getenv(envEndpoint), os.Getenv(legacyEnvEndpoint))
	secret := firstNonEmpty(
		readSecretFile(os.Getenv(envSecretFile), logger),
		os.Getenv(envSecret),
		readSecretFile(os.Getenv(legacyEnvSecretFile), logger),
		os.Getenv(legacyEnvSecret),
	)
	c := &Client{
		endpoint: endpoint,
		secret:   secret,
		timeout:  defaultTimeout,
		http:     &http.Client{Timeout: defaultTimeout},
		logger:   logger,
	}
	if !c.enabled() {
		logger.Warn("sitemap revalidate disabled: endpoint or secret missing",
			zap.Bool("endpoint_loaded", endpoint != ""),
			zap.Bool("secret_loaded", secret != ""))
	}
	return c
}

// enabled 判定 client 是否处于可调用状态。
func (c *Client) enabled() bool {
	return c != nil && c.endpoint != "" && c.secret != "" && c.http != nil
}

// readSecretFile 从指定文件读取共享密钥并去除首尾空白
func readSecretFile(path string, logger *zap.Logger) string {
	if path == "" {
		return ""
	}
	content, err := os.ReadFile(path)
	if err != nil {
		logger.Warn("sitemap revalidate secret file read failed", zap.String("path", path), zap.Error(err))
		return ""
	}
	return strings.TrimSpace(string(content))
}

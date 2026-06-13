package revalidator

import (
	"net/http"
	"os"
	"time"

	"go.uber.org/zap"
)

// 默认配置常量。
const (
	// defaultTimeout 调用 Nuxt 失效接口的整体 HTTP 超时。
	defaultTimeout = 3 * time.Second

	// envEndpoint Nuxt 失效接口完整 URL 所在的环境变量名。
	// 例如：http://portal-web:3000/api/_revalidate
	// 留空时 client 会自动退化为 noop，便于本地不依赖 Nuxt 也能跑后端。
	envEndpoint = "NUXT_REVALIDATE_ENDPOINT"

	// envSecret 共享密钥所在的环境变量名。
	// 与 portal-web 侧 NUXT_REVALIDATE_SECRET 保持一致；任一为空都会退化为 noop。
	envSecret = "NUXT_REVALIDATE_SECRET"
)

// Client 用来调用 Nuxt 失效接口。
//
// 实例由 DI 容器构造，单例复用 http.Client（内部连接池）。
// 当 endpoint 或 secret 任一为空，所有失效调用立即返回，不发起任何 HTTP 调用。
type Client struct {
	endpoint string
	secret   string
	timeout  time.Duration
	http     *http.Client
	logger   *zap.Logger
}

func New(logger *zap.Logger) *Client {
	endpoint := os.Getenv(envEndpoint)
	secret := os.Getenv(envSecret)
	c := &Client{
		endpoint: endpoint,
		secret:   secret,
		timeout:  defaultTimeout,
		http:     &http.Client{Timeout: defaultTimeout},
		logger:   logger,
	}
	if !c.enabled() {
		logger.Warn("validator disabled: endpoint or secret missing",
			zap.String("endpoint_env", envEndpoint),
			zap.Bool("endpoint_loaded", endpoint != ""),
			zap.String("secret_env", envSecret),
			zap.Bool("secret_loaded", secret != ""))
	}
	return c
}

// enabled 判定 client 是否处于"可用"状态，避免对外暴露内部字段。
func (c *Client) enabled() bool {
	return c != nil && c.endpoint != "" && c.secret != ""
}

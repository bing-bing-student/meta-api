package sitemap

import (
	"context"
	"net/http"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"

	"meta-api/common/env"
	"meta-api/common/utils"
)

// 默认配置常量。
const (
	// defaultTimeout 调用 portal-web sitemap 刷新接口的整体 HTTP 超时。
	defaultTimeout = 3 * time.Second
)

// Client 用来调用 portal-web /api/_revalidate 刷新 sitemap 内部缓存。
type Client struct {
	endpoint string
	secret   string
	timeout  time.Duration
	http     *http.Client
	logger   *zap.Logger
	ctx      context.Context
}

// New 构造 sitemap 刷新客户端。
func New(logger *zap.Logger, ctx context.Context) *Client {
	endpoint := strings.TrimSpace(os.Getenv(env.SitemapRevalidateEndpoint))
	secret, err := utils.EnvOrFile(env.SitemapRevalidateSecret)
	if err != nil {
		logger.Warn("sitemap revalidate disabled: secret file read failed", zap.Error(err))
	}
	c := &Client{
		endpoint: endpoint,
		secret:   secret,
		timeout:  defaultTimeout,
		http:     &http.Client{Timeout: defaultTimeout},
		logger:   logger,
		ctx:      ctx,
	}
	if !c.enabled() {
		logger.Warn("sitemap revalidate disabled: endpoint or secret missing", zap.Bool("endpoint_loaded", endpoint != ""), zap.Bool("secret_loaded", secret != ""))
	}
	return c
}

// enabled 判定 client 是否处于可调用状态。
func (c *Client) enabled() bool {
	return c != nil && c.endpoint != "" && c.secret != "" && c.http != nil
}

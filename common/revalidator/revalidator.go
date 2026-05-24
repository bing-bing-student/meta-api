// Package revalidator 负责把"文章变更"通知给前端 Nuxt 容器，让它失效对应的 ISR 缓存。
//
// 设计原则：
//   - 永不阻塞业务主流程：所有 HTTP 调用走 goroutine + 超时 context；
//   - 永不向上抛错：失败只打 Warn 日志，TTL 兜底（Nuxt 侧默认 7 天）；
//   - 永不影响本地开发：当 Endpoint 或 Secret 为空时退化为 noop；
//   - 与前端契约对齐：参见 portal-web 仓库 .dbg/revalidate-contract.md。
package revalidator

import (
	"bytes"
	"context"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/bytedance/sonic"
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
// 当 endpoint 或 secret 任一为空，Revalidate 立即返回，不发起任何 HTTP 调用。
type Client struct {
	endpoint string
	secret   string
	timeout  time.Duration
	http     *http.Client
	logger   *zap.Logger
}

// New 构造一个 Revalidator 客户端。
//
// 与 mysql / redis 的连接信息保持同一风格：endpoint 与 secret 都从环境变量读取，
// 不写入 config.yml。生产环境通过 docker secrets 落到 /run/secrets/* 后由
// bootstrap/app.go 的 init() 自动注入到 os.Getenv。
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
		logger.Warn("revalidator disabled: endpoint or secret missing",
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

// Revalidate 异步通知 Nuxt 失效给定路径列表，永不阻塞调用方。
//
// 重复路径不做去重（Nuxt 侧本身幂等，对一个不存在的 key 调 remove 也是 0 cost）。
// 空切片或 client 未启用时直接返回，调用方无需自行判空。
func (c *Client) Revalidate(paths ...string) {
	if !c.enabled() || len(paths) == 0 {
		return
	}
	// 在拷贝 slice 后再启动 goroutine，避免被调用方后续修改影响。
	cloned := make([]string, len(paths))
	copy(cloned, paths)
	go c.do(map[string]any{"paths": cloned})
}

// do 实际发起 HTTP 调用。失败仅记录 Warn 日志。
func (c *Client) do(payload map[string]any) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	body, err := sonic.Marshal(payload)
	if err != nil {
		c.logger.Warn("revalidate marshal payload failed", zap.Error(err))
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		c.logger.Warn("revalidate build request failed", zap.Error(err))
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-revalidate-secret", c.secret)

	resp, err := c.http.Do(req)
	if err != nil {
		c.logger.Warn("revalidate call failed", zap.Any("payload", payload), zap.Error(err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.logger.Warn("revalidate non-200",
			zap.Int("status", resp.StatusCode),
			zap.Any("payload", payload))
		return
	}
	c.logger.Info("revalidate ok", zap.Any("payload", payload))
}

// ArticleDetailPath 返回文章详情页对应的前端路径。
//
// 与 portal-web 路由 pages/article-detail/[id].vue 对齐。
func ArticleDetailPath(articleID string) string {
	return "/article-detail/" + articleID
}

// HomePath 首页路径。
func HomePath() string {
	return "/"
}

// TagPath 返回标签筛选页对应的前端路径。
//
// 与 portal-web 路由 pages/tag.vue 对齐：标签通过 query string 传递。
// 这里使用 url.QueryEscape 处理含中文 / 空格的标签名，避免被前端 trim 出错。
func TagPath(tagName string) string {
	return "/tag?tagName=" + url.QueryEscape(tagName)
}

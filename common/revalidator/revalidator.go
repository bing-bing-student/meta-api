// Package revalidator 负责把"文章变更"通知给前端 Nuxt 容器，让它失效对应的 ISR 缓存。
//
// 设计原则：
//   - 永不阻塞业务主流程：所有 HTTP 调用走 goroutine + 超时 context；
//   - 永不向上抛错：失败只打 Warn 日志，TTL 兜底（Nuxt 侧默认 7 天）；
//   - 永不影响本地开发：当 Endpoint 或 Secret 为空时退化为 noop；
//   - 与前端契约对齐：参见 portal-web 仓库 server/api/_revalidate.post.ts。
//
// 范围说明：
//
//	前端目前仅对 /article-detail/<id> 启用了 ISR 缓存（其它页面 SSR 走源站），
//	因此本包只暴露面向"文章详情"的语义接口；其它路径调过去前端会被忽略，
//	没有意义，所以也不开放调用入口，避免误用。
package revalidator

import (
	"bytes"
	"context"
	"net/http"
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

	// articleDetailPathPrefix 与 portal-web 路由 pages/article-detail/[id].vue 对齐，
	// 前端 _revalidate.post.ts 的 pathToCacheKeys 用 /article-detail/<id> 正则解析，
	// 必须严格保持该形态：不带 query、不带 hash。
	articleDetailPathPrefix = "/article-detail/"
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

// RevalidateArticles 异步通知 Nuxt 失效给定文章详情页的 ISR 缓存，永不阻塞调用方。
//
// 入参为文章主键 ID 列表（即雪花 ID 的字符串形式）；包内部按前端契约拼成
// /article-detail/<id> 路径再发出。空 ID 会被丢弃，避免拼出 /article-detail/。
//
// 重复 ID 不做去重（前端 _revalidate 是幂等的，对不存在的 key removeItem 也是 0 cost）。
// 空切片或 client 未启用时直接返回，调用方无需自行判空。
func (c *Client) RevalidateArticles(articleIDs ...string) {
	if !c.enabled() || len(articleIDs) == 0 {
		return
	}
	paths := make([]string, 0, len(articleIDs))
	for _, id := range articleIDs {
		if id == "" {
			continue
		}
		paths = append(paths, articleDetailPathPrefix+id)
	}
	if len(paths) == 0 {
		return
	}
	go c.do(map[string]any{"paths": paths})
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

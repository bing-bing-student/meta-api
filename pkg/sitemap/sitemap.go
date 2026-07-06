package sitemap

import (
	"bytes"
	"context"
	"net/http"

	"github.com/bytedance/sonic"
	"go.uber.org/zap"
)

// RefreshArticles 异步通知 portal-web 清理给定文章关联的 sitemap 缓存。
func (c *Client) RefreshArticles(articleIDs ...string) {
	if !c.enabled() || len(articleIDs) == 0 {
		return
	}
	paths := make([]string, 0, len(articleIDs))
	for _, id := range articleIDs {
		path := articleDetailPath(id)
		if path == "" {
			continue
		}
		paths = append(paths, path)
	}
	if len(paths) == 0 {
		return
	}
	go c.do(revalidatePayload{Paths: paths})
}

// do 实际发起 sitemap 刷新 HTTP 调用，失败只记录日志。
func (c *Client) do(payload revalidatePayload) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	body, err := sonic.Marshal(payload)
	if err != nil {
		c.logger.Warn("sitemap revalidate marshal payload failed", zap.Error(err))
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		c.logger.Warn("sitemap revalidate build request failed", zap.Error(err))
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-revalidate-secret", c.secret)

	resp, err := c.http.Do(req)
	if err != nil {
		c.logger.Warn("sitemap revalidate call failed", zap.Strings("paths", payload.Paths), zap.Error(err))
		return
	}
	defer func() {
		if err = resp.Body.Close(); err != nil {
			c.logger.Debug("sitemap revalidate response body close failed", zap.Error(err))
		}
	}()

	if resp.StatusCode != http.StatusOK {
		c.logger.Warn("sitemap revalidate non-200",
			zap.Int("status", resp.StatusCode),
			zap.Strings("paths", payload.Paths))
		return
	}
}

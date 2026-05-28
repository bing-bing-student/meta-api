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
//
// 文件分布：
//
//	revalidator.go  —— Package doc + 业务入口 RevalidateArticles + HTTP 发送逻辑
//	client.go       —— Client 结构、构造与可用性判定（含 env 读取）
//	types.go        —— 与前端契约对齐的请求体结构
//	utils.go        —— 文章 ID → 前端路径 的拼接帮手
package revalidator

import (
	"bytes"
	"context"
	"net/http"

	"github.com/bytedance/sonic"
	"go.uber.org/zap"
)

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
		p := articleDetailPath(id)
		if p == "" {
			continue
		}
		paths = append(paths, p)
	}
	if len(paths) == 0 {
		return
	}
	go c.do(revalidatePayload{Paths: paths})
}

// do 实际发起 HTTP 调用。失败仅记录 Warn 日志。
func (c *Client) do(payload revalidatePayload) {
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
		c.logger.Warn("revalidate call failed", zap.Strings("paths", payload.Paths), zap.Error(err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.logger.Warn("revalidate non-200",
			zap.Int("status", resp.StatusCode),
			zap.Strings("paths", payload.Paths))
		return
	}
	c.logger.Info("revalidate ok", zap.Strings("paths", payload.Paths))
}

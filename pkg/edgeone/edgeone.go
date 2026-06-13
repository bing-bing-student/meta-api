package edgeone

import (
	"context"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	teo "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/teo/v20220901"
	"go.uber.org/zap"
)

// purgeType 与 EdgeOne CreatePurgeTask 接口的 Type 字段对齐。
// purge_prefix 表示前缀刷新，会清除目标 URL 前缀下的所有资源（含任意 query string）。
const purgeType = "purge_prefix"

// purgeMethod EdgeOne 提供两种清理方式：
//
//	delete     —— 直接删，下次访问触发回源拉取（强一致，本场景采用）
//	invalidate —— 标记过期，下次访问发条件请求让源站决定是否更新
//
// 业务诉求是"立即看到最新内容"，所以选 delete。
const purgeMethod = "delete"

// PurgeArticles 异步清理 EdgeOne 上指定文章详情页的 CDN 缓存，永不阻塞调用方。
//
// 入参为文章主键 ID 列表（即雪花 ID 的字符串形式），包内部按
// `<domain>/article-detail/<id>/` 拼成前缀清理 target，
// 一次提交可覆盖详情页 HTML、_payload.json 及任意附属资源（含带 build-hash 的 query）。
//
// 重复 ID 不做去重（接口幂等，且每日配额对个人版足够）。
// 空切片或 client 未启用时直接返回，调用方无需自行判空。
func (c *Client) PurgeArticles(articleIDs ...string) {
	if !c.enabled() || len(articleIDs) == 0 {
		return
	}
	targets := make([]string, 0, len(articleIDs))
	for _, id := range articleIDs {
		t := articleDetailPrefixURL(c.domain, id)
		if t == "" {
			continue
		}
		targets = append(targets, t)
	}
	if len(targets) == 0 {
		return
	}
	go c.do(targets)
}

// do 实际发起清缓存调用。失败仅记录 Warn 日志。
func (c *Client) do(targets []string) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	req := teo.NewCreatePurgeTaskRequest()
	req.ZoneId = common.StringPtr(c.zoneID)
	req.Type = common.StringPtr(purgeType)
	req.Method = common.StringPtr(purgeMethod)
	req.Targets = common.StringPtrs(targets)

	resp, err := c.purger.CreatePurgeTaskWithContext(ctx, req)
	if err != nil {
		c.logger.Warn("edgeOne purge call failed",
			zap.Strings("targets", targets), zap.Error(err))
		return
	}
	if resp == nil || resp.Response == nil {
		c.logger.Warn("edgeOne purge empty response", zap.Strings("targets", targets))
		return
	}
	if len(resp.Response.FailedList) > 0 {
		c.logger.Warn("edgeOne purge partial failed",
			zap.Strings("targets", targets),
			zap.Int("failed_count", len(resp.Response.FailedList)),
			zap.String("request_id", derefString(resp.Response.RequestId)))
		return
	}
	c.logger.Info("edgeOne purge ok",
		zap.Strings("targets", targets),
		zap.String("job_id", derefString(resp.Response.JobId)),
		zap.String("request_id", derefString(resp.Response.RequestId)))
}

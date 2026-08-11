package edgeone

import (
	"context"
	"fmt"
	"time"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	teo "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/teo/v20220901"
	"go.uber.org/zap"

	"meta-api/common/utils"
)

// purgeType 与 EdgeOne CreatePurgeTask 接口的 Type 字段对齐。
// purge_url 表示精确 URL 刷新；官方语义是直接删除匹配 URL 的节点缓存。
const purgeType = "purge_url"

const (
	purgeJobIDFilterName = "job-id"

	purgeStatusProcessing = "processing"
	purgeStatusSuccess    = "success"
	purgeStatusFailed     = "failed"
	purgeStatusTimeout    = "timeout"
	purgeStatusCanceled   = "canceled"
)

// PurgeArticles 同步清理 EdgeOne 上指定文章详情页的 CDN 缓存。
//
// 入参为文章主键 ID 列表（即雪花 ID 的字符串形式），包内部按
// `<domain>/article-detail/<id>` 拼成精确 URL 清理 target。
//
// 重复 ID 不做去重（接口幂等，且每日配额对个人版足够）。
// 空切片直接返回 nil；非生产环境 client 未启用时也返回 nil，避免本地开发被腾讯云 env 阻断。
// 生产环境 client 未启用会返回错误，避免文章变更在没有实际清 CDN 的情况下被提示成功。
func (c *Client) PurgeArticles(articleIDs ...string) error {
	if len(articleIDs) == 0 {
		return nil
	}
	if !c.enabled() {
		if utils.IsProductionEnv() {
			return fmt.Errorf("edgeOne purge disabled in production")
		}
		return nil
	}
	targets := make([]string, 0, len(articleIDs))
	for _, id := range articleIDs {
		t := articleDetailURL(c.domain, id)
		if t == "" {
			continue
		}
		targets = append(targets, t)
	}
	if len(targets) == 0 {
		return nil
	}
	return c.do(targets)
}

// do 实际发起清缓存调用。
func (c *Client) do(targets []string) error {
	ctx, cancel := context.WithTimeout(c.ctx, c.waitTimeout)
	defer cancel()

	jobID, err := c.createPurgeTask(ctx, targets)
	if err != nil {
		return err
	}
	return c.waitPurgeTask(ctx, jobID, targets)
}

func (c *Client) createPurgeTask(ctx context.Context, targets []string) (string, error) {
	req := teo.NewCreatePurgeTaskRequest()
	req.ZoneId = common.StringPtr(c.zoneID)
	req.Type = common.StringPtr(purgeType)
	req.Targets = common.StringPtrs(targets)

	resp, err := c.purger.CreatePurgeTaskWithContext(ctx, req)
	if err != nil {
		c.logger.Warn("edgeOne purge call failed", zap.Strings("targets", targets), zap.Error(err))
		return "", fmt.Errorf("edgeOne purge call failed: %w", err)
	}
	if resp == nil || resp.Response == nil {
		c.logger.Warn("edgeOne purge empty response", zap.Strings("targets", targets))
		return "", fmt.Errorf("edgeOne purge empty response")
	}
	if len(resp.Response.FailedList) > 0 {
		c.logger.Warn("edgeOne purge partial failed", zap.Strings("targets", targets), zap.Int("failed_count", len(resp.Response.FailedList)))
		return "", fmt.Errorf("edgeOne purge partial failed: failed_count=%d", len(resp.Response.FailedList))
	}
	jobID := ptrValue(resp.Response.JobId)
	if jobID == "" {
		c.logger.Warn("edgeOne purge empty job id", zap.Strings("targets", targets))
		return "", fmt.Errorf("edgeOne purge empty job id")
	}
	return jobID, nil
}

func (c *Client) waitPurgeTask(ctx context.Context, jobID string, targets []string) error {
	ticker := time.NewTicker(c.pollInterval)
	defer ticker.Stop()

	for {
		status, err := c.describePurgeTaskStatus(ctx, jobID)
		if err != nil {
			return err
		}

		switch status {
		case purgeStatusSuccess:
			return nil
		case purgeStatusFailed, purgeStatusTimeout, purgeStatusCanceled:
			c.logger.Warn("edgeOne purge task failed",
				zap.String("job_id", jobID),
				zap.String("status", status),
				zap.Strings("targets", targets))
			return fmt.Errorf("edgeOne purge task failed: job_id=%s status=%s", jobID, status)
		}

		select {
		case <-ctx.Done():
			c.logger.Warn("edgeOne purge task wait timeout",
				zap.String("job_id", jobID),
				zap.String("status", status),
				zap.Strings("targets", targets),
				zap.Error(ctx.Err()))
			return fmt.Errorf("edgeOne purge task wait timeout: job_id=%s: %w", jobID, ctx.Err())
		case <-ticker.C:
		}
	}
}

func (c *Client) describePurgeTaskStatus(ctx context.Context, jobID string) (string, error) {
	limit := int64(1)
	req := teo.NewDescribePurgeTasksRequest()
	req.ZoneId = common.StringPtr(c.zoneID)
	req.Limit = &limit
	req.Filters = []*teo.AdvancedFilter{
		{
			Name:   common.StringPtr(purgeJobIDFilterName),
			Values: common.StringPtrs([]string{jobID}),
		},
	}

	resp, err := c.purger.DescribePurgeTasksWithContext(ctx, req)
	if err != nil {
		c.logger.Warn("edgeOne describe purge task failed", zap.String("job_id", jobID), zap.Error(err))
		return "", fmt.Errorf("edgeOne describe purge task failed: %w", err)
	}
	if resp == nil || resp.Response == nil {
		c.logger.Warn("edgeOne describe purge task empty response", zap.String("job_id", jobID))
		return "", fmt.Errorf("edgeOne describe purge task empty response")
	}
	if len(resp.Response.Tasks) == 0 || resp.Response.Tasks[0] == nil {
		return purgeStatusProcessing, nil
	}
	status := ptrValue(resp.Response.Tasks[0].Status)
	if status == "" {
		return purgeStatusProcessing, nil
	}
	return status, nil
}

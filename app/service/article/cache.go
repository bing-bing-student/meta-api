package article

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"

	"meta-api/app/model/article"
	"meta-api/common/cachekey"
	"meta-api/common/constants"
)

// 缓存预热与持久化使用的批处理大小与超时配置
const (
	warmUpBatchSize = 1000

	// cronJobTimeout 单次定时落盘任务的最大执行时长
	cronJobTimeout = 30 * time.Second
)

// WarmUpCache 启动时预热文章 ZSet 缓存：清空旧数据并按时间/浏览量重新构建
// 使用 Pipeline + 分批写入，将 N 次 RTT 压缩为 ⌈N/batch⌉ 次
func (a *articleService) WarmUpCache(ctx context.Context) error {
	timeKey := cachekey.ArticleTimeZSet().String()
	viewKey := cachekey.ArticleViewZSet().String()
	if err := a.redis.Del(ctx, timeKey, viewKey).Err(); err != nil {
		a.logger.Error("failed to clear article zset", zap.Error(err))
		return fmt.Errorf("failed to clear article zset: %w", err)
	}

	list, err := a.articleModel.ListTimeAndView(ctx)
	if err != nil {
		a.logger.Error("failed to list articles for warm up", zap.Error(err))
		return err
	}
	if len(list) == 0 {
		return nil
	}

	for start := 0; start < len(list); start += warmUpBatchSize {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		end := start + warmUpBatchSize
		if end > len(list) {
			end = len(list)
		}

		timeMembers := make([]redis.Z, 0, end-start)
		viewMembers := make([]redis.Z, 0, end-start)
		for _, d := range list[start:end] {
			timeMembers = append(timeMembers, redis.Z{
				Score:  cachekey.ArticleTimeScore(d.CreateTime),
				Member: d.ID,
			})
			viewMembers = append(viewMembers, redis.Z{
				Score:  cachekey.ArticleViewScore(d.ViewNum),
				Member: d.ID,
			})
		}

		pipe := a.redis.Pipeline()
		pipe.ZAdd(ctx, timeKey, timeMembers...)
		pipe.ZAdd(ctx, viewKey, viewMembers...)
		if _, err = pipe.Exec(ctx); err != nil {
			a.logger.Error("failed to warm up article zset",
				zap.Int("start", start), zap.Int("end", end), zap.Error(err))
			return fmt.Errorf("failed to warm up article zset: %w", err)
		}
	}

	a.logger.Info("article cache warmed up", zap.Int("total", len(list)))
	return nil
}

// PersistViewCount 关闭时把 Redis 中的浏览量批量回写到数据库
// 通过单条 CASE WHEN UPDATE 完成，避免 N 次 RTT 拖慢关闭流程
func (a *articleService) PersistViewCount(ctx context.Context) error {
	list, err := a.redis.ZRangeWithScores(ctx, cachekey.ArticleViewZSet().String(), 0, -1).Result()
	if err != nil {
		a.logger.Error("failed to query article view zset", zap.Error(err))
		return fmt.Errorf("failed to query article view zset: %w", err)
	}
	if len(list) == 0 {
		return nil
	}

	items := make([]article.ViewNumUpdate, 0, len(list))
	for _, element := range list {
		id, ok := toIDString(element.Member)
		if !ok {
			a.logger.Warn("unexpected zset member type", zap.Any("member", element.Member))
			continue
		}
		items = append(items, article.ViewNumUpdate{
			ID:      id,
			ViewNum: int(element.Score),
		})
	}

	if err = a.articleModel.BatchUpdateViewNum(ctx, items); err != nil {
		a.logger.Error("failed to persist article view num", zap.Error(err))
		return err
	}

	a.logger.Info("article view num persisted", zap.Int("total", len(items)))
	return nil
}

// toIDString 兼容 ZSet member 可能为 string / 数值类型
func toIDString(member any) (string, bool) {
	switch v := member.(type) {
	case string:
		return v, true
	case int64:
		return strconv.FormatInt(v, 10), true
	case uint64:
		return strconv.FormatUint(v, 10), true
	default:
		return "", false
	}
}

// RegisterCronJobs 把文章相关的定时任务注册到外部 cron 调度器
// 由 app 层在启动阶段调用，调用方负责在退出时通过返回的 entryID 反注册
func (a *articleService) RegisterCronJobs(c *cron.Cron) ([]cron.EntryID, error) {
	entryID, err := c.AddFunc(constants.Spec, func() {
		// 每次 cron 触发都使用独立的超时 ctx，避免长任务卡住调度器
		ctx, cancel := context.WithTimeout(context.Background(), cronJobTimeout)
		defer cancel()

		if err := a.PersistViewCount(ctx); err != nil {
			a.logger.Error("cron persist view count failed", zap.Error(err))
		}
	})
	if err != nil {
		return nil, fmt.Errorf("failed to register article cron jobs: %w", err)
	}
	a.logger.Info("article cron jobs registered", zap.String("spec", constants.Spec))
	return []cron.EntryID{entryID}, nil
}

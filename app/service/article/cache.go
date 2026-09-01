package article

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
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
)

// WarmUpCache 启动时预热文章 ZSet 缓存。
// 新数据先分批写入本次预热专用的临时 Key，全部成功后再通过
// Redis MULTI/EXEC 同时切换时间和浏览量两个正式 Key。构建失败或取消时，
// 读请求始终看到上一个完整版本，不会暴露空集合或半成品。
func (a *articleService) WarmUpCache(ctx context.Context) error {
	timeKey := cachekey.ArticleTimeZSet().String()
	viewKey := cachekey.ArticleViewZSet().String()
	list, err := a.articleModel.ListTimeAndView(ctx)
	if err != nil {
		a.logger.Error("failed to list articles for warm up", zap.Error(err))
		return err
	}

	suffix := uuid.NewString()
	tempTimeKey := timeKey + ":warming:" + suffix
	tempViewKey := viewKey + ":warming:" + suffix
	const sentinel = "__cache_warmup_sentinel__"
	const temporaryKeyTTL = time.Hour
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if cleanupErr := a.redis.Del(cleanupCtx, tempTimeKey, tempViewKey).Err(); cleanupErr != nil {
			a.logger.Warn("failed to clean temporary article cache",
				zap.String("time_key", tempTimeKey), zap.String("view_key", tempViewKey), zap.Error(cleanupErr))
		}
	}()

	// Redis 不保留空 ZSet，哨兵成员确保即使 MySQL 无文章，临时 Key
	// 也存在并可被 RENAME；切换事务中会立即删除哨兵。
	seedPipe := a.redis.Pipeline()
	seedPipe.ZAdd(ctx, tempTimeKey, redis.Z{Score: 0, Member: sentinel})
	seedPipe.ZAdd(ctx, tempViewKey, redis.Z{Score: 0, Member: sentinel})
	seedPipe.Expire(ctx, tempTimeKey, temporaryKeyTTL)
	seedPipe.Expire(ctx, tempViewKey, temporaryKeyTTL)
	if _, err = seedPipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to initialize temporary article cache: %w", err)
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
		pipe.ZAdd(ctx, tempTimeKey, timeMembers...)
		pipe.ZAdd(ctx, tempViewKey, viewMembers...)
		if _, err = pipe.Exec(ctx); err != nil {
			a.logger.Error("failed to warm up article ZSet",
				zap.Int("start", start), zap.Int("end", end), zap.Error(err))
			return fmt.Errorf("failed to warm up article ZSet: %w", err)
		}
	}

	// 两个 RENAME 与哨兵清理在同一个 Redis 事务中执行，对外只暴露
	// 旧版本或新版本，不会出现两个索引版本不一致。
	if _, err = a.redis.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.Rename(ctx, tempTimeKey, timeKey)
		pipe.Rename(ctx, tempViewKey, viewKey)
		pipe.ZRem(ctx, timeKey, sentinel)
		pipe.ZRem(ctx, viewKey, sentinel)
		return nil
	}); err != nil {
		a.logger.Error("failed to switch article cache version", zap.Error(err))
		return fmt.Errorf("failed to switch article cache version: %w", err)
	}

	a.logger.Info("article cache warmed up", zap.Int("total", len(list)))
	return nil
}

// PersistViewCount 关闭时把 Redis 中的浏览量批量回写到数据库
// 通过单条 CASE WHEN UPDATE 完成，避免 N 次 RTT 拖慢关闭流程
func (a *articleService) PersistViewCount(ctx context.Context) error {
	list, err := a.redis.ZRangeWithScores(ctx, cachekey.ArticleViewZSet().String(), 0, -1).Result()
	if err != nil {
		a.logger.Error("failed to query article view ZSet", zap.Error(err))
		return fmt.Errorf("failed to query article view ZSet: %w", err)
	}
	if len(list) == 0 {
		return nil
	}

	items := make([]article.ViewNumUpdate, 0, len(list))
	for _, element := range list {
		id, ok := toIDString(element.Member)
		if !ok {
			a.logger.Warn("unexpected ZSet member type", zap.Any("member", element.Member))
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
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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

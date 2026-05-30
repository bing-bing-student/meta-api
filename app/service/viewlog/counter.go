package viewlog

import (
	"context"

	"go.uber.org/zap"

	"meta-api/common/cachekey"
)

// increment 通过 Redis 完成"浏览量 +1"：
//   - HINCRBY article:{id}:Hash viewNum 1   —— 文章详情页与列表页直接读这个 Hash
//   - ZINCRBY article:view:ZSet 1 articleID —— 热门文章列表 / 搜索结果浏览量校正
//
// MySQL 由后台 cron PersistViewCount 周期性把 ZSet 里的增量回写。
// 任一调用失败仅打 Warn 日志：响应不应因计数失败而暴露差异。
func (s *viewLogService) increment(ctx context.Context, articleID string) {
	pipe := s.redis.Pipeline()
	pipe.HIncrBy(ctx, cachekey.ArticleHash(articleID).String(), "viewNum", 1)
	pipe.ZIncrBy(ctx, cachekey.ArticleViewZSet().String(), 1, articleID)
	if _, err := pipe.Exec(ctx); err != nil {
		s.logger.Warn("view-log increment failed",
			zap.String("article_id", articleID), zap.Error(err))
	}
}

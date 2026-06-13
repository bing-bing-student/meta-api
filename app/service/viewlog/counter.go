package viewlog

import (
	"context"

	"go.uber.org/zap"

	"meta-api/common/cachekey"
)

// Increment 通过 Redis 完成"浏览量 +1"
func (s *viewLogService) Increment(ctx context.Context, articleID string) {
	pipe := s.redis.Pipeline()
	pipe.HIncrBy(ctx, cachekey.ArticleHash(articleID).String(), "viewNum", 1)
	pipe.ZIncrBy(ctx, cachekey.ArticleViewZSet().String(), 1, articleID)
	if _, err := pipe.Exec(ctx); err != nil {
		s.logger.Warn("view-log increment failed",
			zap.String("article_id", articleID), zap.Error(err))
	}
}

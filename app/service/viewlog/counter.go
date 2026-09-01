package viewlog

import (
	"context"
	"time"

	"go.uber.org/zap"

	"meta-api/common/cachekey"
)

const incrementTimeout = time.Second

// Increment 通过 Redis 完成"浏览量 +1"
func (s *viewLogService) Increment(articleID string) {
	ctx, cancel := context.WithTimeout(context.Background(), incrementTimeout)
	defer cancel()

	// 浏览量只维护在 ZSet 中。不能对文章详情 Hash 直接 HINCRBY：当详情
	// 缓存尚未建立或刚被失效时，HINCRBY 会创建一个仅含 viewNum 的残缺
	// Hash，后续列表把它当成完整缓存读取会得到 nil 字段甚至触发 panic。
	if err := s.redis.ZIncrBy(ctx, cachekey.ArticleViewZSet().String(), 1, articleID).Err(); err != nil {
		s.logger.Warn("view-log increment failed", zap.String("article_id", articleID), zap.Error(err))
	}
}

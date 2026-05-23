package cachekey

import "time"

// 文章缓存命名空间与排序维度
const (
	nsArticle = "article"

	// ZSet 排序维度（也是请求参数 request.Order 的合法值）
	OrderTime = "time"
	OrderView = "view"
)

// ArticleTimeZSet 全部文章按创建时间排序的有序集合
func ArticleTimeZSet() Key { return build(nsArticle, OrderTime, "ZSet") }

// ArticleViewZSet 全部文章按浏览量排序的有序集合
func ArticleViewZSet() Key { return build(nsArticle, OrderView, "ZSet") }

// ArticleHash 单篇文章详情缓存（Hash 结构）
func ArticleHash(id string) Key { return build(nsArticle, id, "Hash") }

// ArticleOrderZSet 按运行时维度（time / view）选择对应的 ZSet。
// order 必须是合法枚举之一，否则返回 ok = false（避免用户输入污染 Key 命名空间）。
func ArticleOrderZSet(order string) (Key, bool) {
	switch order {
	case OrderTime, OrderView:
		return build(nsArticle, order, "ZSet"), true
	default:
		return "", false
	}
}

// ArticleViewLock 浏览量去重锁的 Key（lua 脚本中用于同一用户在 expireTime 内只计一次）
// 当前格式与历史 Lua 脚本保持一致：{articleID}:{userID}
func ArticleViewLock(articleID, userID string) Key {
	return Key(articleID + ":" + userID)
}

// ArticleTimeScore 文章按创建时间排序的 score（毫秒时间戳）
func ArticleTimeScore(t time.Time) float64 { return float64(t.UnixMilli()) }

// ArticleViewScore 文章按浏览量排序的 score
func ArticleViewScore(viewNum uint64) float64 { return float64(viewNum) }

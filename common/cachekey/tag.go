package cachekey

const nsTag = "tag"

// TagArticleNumZSet 按文章数量排序的标签集合
func TagArticleNumZSet() Key { return build(nsTag, "articleNum", "ZSet") }

// TagArticleListZSet 某标签下的文章 ID 列表，按创建时间排序。
//
// 历史 Key 格式为 "{tagName}:article:ZSet"（前缀直接是 tagName），
// 为了避免重命名导致存量缓存失效，这里保持兼容。
// 后续若要规范命名，可改为 build(nsTag, tagName, "articles", "ZSet")。
func TagArticleListZSet(tagName string) Key {
	return build(tagName, "article", "ZSet")
}

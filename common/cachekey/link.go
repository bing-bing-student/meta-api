package cachekey

const nsLink = "link"

// LinkZSet 友链列表缓存，按更新时间排序。
func LinkZSet() Key {
	return build(nsLink, "ZSet")
}

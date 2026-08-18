package cachekey

const nsLink = "link"
const nsSiteDynamic = "siteDynamic"

// LinkZSet 友链列表缓存，按更新时间排序。
func LinkZSet() Key {
	return build(nsLink, "ZSet")
}

// SiteDynamicPublishedList 前台站点动态整包列表缓存，顺序与后台拖拽顺序一致。
func SiteDynamicPublishedList() Key {
	return build(nsSiteDynamic, "published", "list")
}

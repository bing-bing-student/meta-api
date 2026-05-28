package revalidator

// articleDetailPathPrefix 与 portal-web 路由 pages/article-detail/[id].vue 对齐，
// 前端 _revalidate.post.ts 的 pathToCacheKeys 用 /article-detail/<id> 正则解析，
// 必须严格保持该形态：不带 query、不带 hash。
const articleDetailPathPrefix = "/article-detail/"

// articleDetailPath 把文章 ID 拼成前端可识别的详情页路径。
//
// 入参为雪花 ID 的字符串形式。空 ID 返回空串，调用方负责过滤，
// 避免拼出 /article-detail/ 这种非法路径打到前端。
func articleDetailPath(articleID string) string {
	if articleID == "" {
		return ""
	}
	return articleDetailPathPrefix + articleID
}

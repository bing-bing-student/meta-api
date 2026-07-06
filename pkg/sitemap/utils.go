package sitemap

// articleDetailPathPrefix 文章详情前端路由段，与 portal-web 路由保持一致。
const articleDetailPathPrefix = "/article-detail/"

// articleDetailPath 把文章 ID 拼成 portal-web 可识别的详情页路径。
func articleDetailPath(articleID string) string {
	if articleID == "" {
		return ""
	}
	return articleDetailPathPrefix + articleID
}

// firstNonEmpty 返回第一个非空字符串。
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

package edgeone

// articleDetailRoutePrefix 文章详情前端路由段，与 portal-web
// pages/article-detail/[id].vue 对齐。
const articleDetailRoutePrefix = "/article-detail/"

// articleDetailURL 把文章 ID 拼成 EdgeOne purge_url 接受的精确目标 URL。
//
// 形态固定为 <domain>/article-detail/<id>，与 portal-web 的 Nuxt 路由、
// canonical URL 和 sitemap URL 保持一致。
//
// 入参 domain 必须是带 scheme 的完整前缀且末尾不带斜杠（由 New 统一规整），
// articleID 为空时返回空串，调用方负责过滤，避免拼出 .../article-detail/。
func articleDetailURL(domain, articleID string) string {
	if domain == "" || articleID == "" {
		return ""
	}
	return domain + articleDetailRoutePrefix + articleID
}

func ptrValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

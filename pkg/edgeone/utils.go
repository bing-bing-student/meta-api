package edgeone

// articleDetailPrefixSegment 文章详情前端路由段，与 portal-web
// pages/article-detail/[id].vue 对齐。
const articleDetailPrefixSegment = "/article-detail/"

// articleDetailPrefixURL 把文章 ID 拼成 EdgeOne purge_prefix 接受的目标 URL。
//
// 形态固定为 <domain>/article-detail/<id>，不带尾斜杠才能同时覆盖真实 HTML
// URL、尾斜杠变体和 query string 变体。
//
// 入参 domain 必须是带 scheme 的完整前缀且末尾不带斜杠（由 New 统一规整），
// articleID 为空时返回空串，调用方负责过滤，避免拼出 .../article-detail/。
func articleDetailPrefixURL(domain, articleID string) string {
	if domain == "" || articleID == "" {
		return ""
	}
	return domain + articleDetailPrefixSegment + articleID
}

// derefString 安全解引用 *string，nil 时返回空串。
// 腾讯云 SDK 全员用 *string 表示可空字段，记日志时需要先取值。
func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

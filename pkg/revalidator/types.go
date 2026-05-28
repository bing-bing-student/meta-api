package revalidator

// revalidatePayload 是发往 Nuxt /api/_revalidate 接口的请求体。
//
// 与 portal-web 侧 server/api/_revalidate.post.ts 的契约对齐：
// 仅包含 paths 字段，每个元素必须是 /article-detail/<id> 形态，
// 不带 query、不带 hash，否则前端 pathToCacheKeys 解析时会被静默丢弃。
type revalidatePayload struct {
	Paths []string `json:"paths"`
}

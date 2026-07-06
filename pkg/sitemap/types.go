package sitemap

// revalidatePayload 是发往 portal-web /api/_revalidate 的请求体。
type revalidatePayload struct {
	Paths []string `json:"paths"`
}

package cachekey

const nsComment = "comment"

// CommentArticleApprovedList 单篇文章已审核评论列表缓存。
func CommentArticleApprovedList(articleID string) Key {
	return build(nsComment, "article", articleID, "approved", "list")
}

// CommentRateLimit 前台评论相关限流 Key。
func CommentRateLimit(parts ...string) Key {
	return build(append([]string{nsComment, "rate-limit"}, parts...)...)
}

// CommentModeration 前台评论审核相关 Key。
func CommentModeration(parts ...string) Key {
	return build(append([]string{nsComment, "moderation"}, parts...)...)
}

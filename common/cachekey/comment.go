package cachekey

const nsComment = "comment"

// CommentRateLimit 前台评论相关限流 Key。
func CommentRateLimit(parts ...string) Key {
	return build(append([]string{nsComment, "rate-limit"}, parts...)...)
}

// CommentModeration 前台评论审核相关 Key。
func CommentModeration(parts ...string) Key {
	return build(append([]string{nsComment, "moderation"}, parts...)...)
}

// CommentApprovedArticle 保存某篇文章的全部已通过评论快照。
// 分页和父子关系由 Service 在内存中组装，后台变更后主动删除。
func CommentApprovedArticle(articleID string) Key {
	return build(nsComment, "approved", "article", articleID)
}

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

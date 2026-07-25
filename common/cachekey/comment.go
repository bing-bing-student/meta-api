package cachekey

const nsComment = "comment"

// CommentArticleApprovedList 单篇文章已审核评论列表缓存。
func CommentArticleApprovedList(articleID string) Key {
	return build(nsComment, "article", articleID, "approved", "list")
}

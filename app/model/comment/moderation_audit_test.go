package comment

import "testing"

func TestModerationTableNamesFollowSingularDatabaseConvention(t *testing.T) {
	if got := (CommentModerationAudit{}).TableName(); got != "comment_moderation_audit" {
		t.Fatalf("audit table = %q", got)
	}
	if got := (CommentModerationFeedback{}).TableName(); got != "comment_moderation_feedback" {
		t.Fatalf("feedback table = %q", got)
	}
}

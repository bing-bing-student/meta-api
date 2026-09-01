package moderation

import (
	"testing"

	appconfig "meta-api/config"
)

func TestFeedbackCategoriesAreDefensiveAndValidated(t *testing.T) {
	cfg := appconfig.CommentModerationConfig{Categories: map[string]appconfig.CommentModerationCategoryConfig{
		"abuse": {FeedbackEnabled: true}, "spam_fraud": {FeedbackEnabled: true},
	}}
	values := FeedbackCategories(cfg)
	if len(values) == 0 || !IsFeedbackCategory("spam_fraud", cfg) || IsFeedbackCategory("anything", cfg) {
		t.Fatalf("unexpected feedback taxonomy: %v", values)
	}
	values[0] = "modified"
	if IsFeedbackCategory("modified", cfg) || FeedbackCategories(cfg)[0] == "modified" {
		t.Fatal("FeedbackCategories must return a defensive copy")
	}
}

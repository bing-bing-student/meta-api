package comment

import (
	"path/filepath"
	"testing"

	"github.com/spf13/viper"

	appconfig "meta-api/config"
)

func newTestCommentModerationConfig(t testing.TB) *appconfig.CommentModerationConfig {
	t.Helper()

	reader := viper.New()
	reader.SetConfigType("yaml")
	reader.SetConfigFile(filepath.Join("..", "..", "..", "config", "comment_moderation.yml"))
	if err := reader.ReadInConfig(); err != nil {
		t.Fatalf("read comment moderation config: %v", err)
	}

	var cfg appconfig.Config
	if err := reader.Unmarshal(&cfg); err != nil {
		t.Fatalf("unmarshal comment moderation config: %v", err)
	}
	if cfg.CommentModerationConfig == nil {
		t.Fatal("missing comment moderation config")
	}
	return cfg.CommentModerationConfig
}

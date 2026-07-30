package bootstrap

import (
	"os"
	"testing"
)

func TestLoadConfigFilesFromSplitFiles(t *testing.T) {
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if chdirErr := os.Chdir(".."); chdirErr != nil {
		t.Fatalf("change working directory: %v", chdirErr)
	}
	defer func() {
		if chdirErr := os.Chdir(workingDirectory); chdirErr != nil {
			t.Fatalf("restore working directory: %v", chdirErr)
		}
	}()

	cfg, files, err := loadConfigFiles()
	if err != nil {
		t.Fatalf("load config files: %v", err)
	}
	if len(files) != len(configFiles) {
		t.Fatalf("expected split config files, got %v", files)
	}
	if cfg.LogConfig == nil || cfg.MySQLConfig == nil || cfg.RedisConfig == nil {
		t.Fatalf("expected app config sections, got %+v", cfg)
	}
	if cfg.RateLimitConfig == nil || cfg.RateLimitConfig.AdminLogin.AccountLogin.IP.Limit == 0 {
		t.Fatalf("expected rate limit config, got %+v", cfg.RateLimitConfig)
	}
	if cfg.BugFeedbackConfig == nil || cfg.BugFeedbackConfig.SMTP.Host != "smtp.qq.com" {
		t.Fatalf("expected bug feedback smtp config, got %+v", cfg.BugFeedbackConfig)
	}
	if cfg.RateLimitConfig.BugFeedback.IP.Limit == 0 {
		t.Fatalf("expected bug feedback rate limit config, got %+v", cfg.RateLimitConfig.BugFeedback)
	}
	if cfg.CommentModerationConfig == nil || cfg.CommentModerationConfig.Lexicon.Provider != "go_swd" {
		t.Fatalf("expected comment moderation config, got %+v", cfg.CommentModerationConfig)
	}
	if len(cfg.CommentModerationConfig.ContextRules) == 0 {
		t.Fatalf("expected comment moderation context rules, got %+v", cfg.CommentModerationConfig)
	}
	if cfg.CommentModerationConfig.Decision.CategoryOverrides["gambling"].Level != "block" {
		t.Fatalf("expected strict moderation decision config, got %+v", cfg.CommentModerationConfig.Decision)
	}
}

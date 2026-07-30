package admin

import (
	"os"
	"path/filepath"
	"testing"

	appconfig "meta-api/config"
)

func TestBugFeedbackSMTPConfigReadsPasswordFile(t *testing.T) {
	secretPath := filepath.Join(t.TempDir(), "smtp_password")
	if err := os.WriteFile(secretPath, []byte(" file-secret \n"), 0600); err != nil {
		t.Fatalf("write smtp password secret: %v", err)
	}

	t.Setenv(bugFeedbackSMTPHostEnv, "")
	t.Setenv(bugFeedbackSMTPPortEnv, "")
	t.Setenv(bugFeedbackSMTPUsernameEnv, "")
	t.Setenv(bugFeedbackSMTPPasswordEnv, "env-secret")
	t.Setenv(bugFeedbackSMTPPasswordFileEnv, secretPath)
	t.Setenv(bugFeedbackSMTPFromEnv, "")
	t.Setenv(bugFeedbackSMTPFromNameEnv, "")

	service := &adminService{
		config: &appconfig.Config{
			BugFeedbackConfig: &appconfig.BugFeedbackConfig{
				SMTP: appconfig.BugFeedbackSMTPConfig{
					Host:     "smtp.example.com",
					Port:     465,
					Username: "feedback@example.com",
					From:     "feedback@example.com",
					FromName: "Feedback",
				},
			},
		},
	}

	cfg, err := service.bugFeedbackSMTPConfig()
	if err != nil {
		t.Fatalf("expected smtp config, got error: %v", err)
	}
	if cfg.Password != "file-secret" {
		t.Fatalf("expected password from secret file, got %q", cfg.Password)
	}
}

func TestBugFeedbackSMTPConfigFallsBackToPasswordEnv(t *testing.T) {
	t.Setenv(bugFeedbackSMTPHostEnv, "")
	t.Setenv(bugFeedbackSMTPPortEnv, "")
	t.Setenv(bugFeedbackSMTPUsernameEnv, "")
	t.Setenv(bugFeedbackSMTPPasswordEnv, "env-secret")
	t.Setenv(bugFeedbackSMTPPasswordFileEnv, "")
	t.Setenv(bugFeedbackSMTPFromEnv, "")
	t.Setenv(bugFeedbackSMTPFromNameEnv, "")

	service := &adminService{
		config: &appconfig.Config{
			BugFeedbackConfig: &appconfig.BugFeedbackConfig{
				SMTP: appconfig.BugFeedbackSMTPConfig{
					Host:     "smtp.example.com",
					Username: "feedback@example.com",
				},
			},
		},
	}

	cfg, err := service.bugFeedbackSMTPConfig()
	if err != nil {
		t.Fatalf("expected smtp config, got error: %v", err)
	}
	if cfg.Password != "env-secret" {
		t.Fatalf("expected password from env, got %q", cfg.Password)
	}
	if cfg.From != "feedback@example.com" {
		t.Fatalf("expected from fallback to username, got %q", cfg.From)
	}
}

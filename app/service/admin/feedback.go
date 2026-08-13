package admin

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"net/mail"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"meta-api/common/cachekey"
	"meta-api/common/ratelimit"
	"meta-api/common/types"
	"meta-api/common/utils"
	appconfig "meta-api/config"
	"meta-api/pkg/mailer"
)

var (
	ErrBugFeedbackInvalid                = errors.New("bug feedback invalid")
	ErrBugFeedbackSMTPNotConfigured      = errors.New("bug feedback smtp not configured")
	ErrBugFeedbackRecipientNotConfigured = errors.New("bug feedback recipient not configured")
)

const (
	bugFeedbackMinMessageLength     = 5
	bugFeedbackMaxMessageLength     = 2000
	bugFeedbackMaxScreenshotBytes   = 2 * 1024 * 1024
	bugFeedbackMaxScreenshotDataURL = 3 * 1024 * 1024
	bugFeedbackMailTimeout          = 8 * time.Second
	defaultBugFeedbackIPLimit       = 3
	defaultBugFeedbackIPWindow      = 10 * time.Minute
	defaultBugFeedbackSMTPPort      = 465
	defaultBugFeedbackSMTPFromName  = "JSON Tool Feedback"
	bugFeedbackSMTPHostEnv          = "BUG_FEEDBACK_SMTP_HOST"
	bugFeedbackSMTPPortEnv          = "BUG_FEEDBACK_SMTP_PORT"
	bugFeedbackSMTPUsernameEnv      = "BUG_FEEDBACK_SMTP_USERNAME"
	bugFeedbackSMTPPasswordEnv      = "BUG_FEEDBACK_SMTP_PASSWORD"
	bugFeedbackSMTPFromEnv          = "BUG_FEEDBACK_SMTP_FROM"
	bugFeedbackSMTPFromNameEnv      = "BUG_FEEDBACK_SMTP_FROM_NAME"
)

type bugFeedbackInvalidError string

func (e bugFeedbackInvalidError) Error() string {
	return string(e)
}

func (e bugFeedbackInvalidError) Is(target error) bool {
	return target == ErrBugFeedbackInvalid
}

func newBugFeedbackInvalid(message string) error {
	return bugFeedbackInvalidError(message)
}

func (a *adminService) UserSubmitBugFeedback(ctx context.Context, request *types.SubmitBugFeedbackRequest) error {
	if request == nil {
		return newBugFeedbackInvalid("无效的请求参数")
	}
	normalizeBugFeedbackRequest(request)
	if err := validateBugFeedbackRequest(request); err != nil {
		return err
	}
	if err := a.checkBugFeedbackLimit(ctx, request.ClientIP); err != nil {
		return err
	}

	recipients, err := a.bugFeedbackRecipients(ctx)
	if err != nil {
		return err
	}
	smtpCfg, err := a.bugFeedbackSMTPConfig()
	if err != nil {
		return err
	}

	attachments := make([]mailer.Attachment, 0, 1)
	if request.ScreenshotDataURL != "" {
		attachment, err := decodeBugFeedbackScreenshot(request.ScreenshotDataURL, request.ScreenshotName)
		if err != nil {
			return err
		}
		attachments = append(attachments, *attachment)
	}

	subject := "JSON 工具 Bug 反馈"
	body := buildBugFeedbackMailBody(request)
	mailCtx, cancel := context.WithTimeout(ctx, bugFeedbackMailTimeout)
	defer cancel()
	if err = mailer.SendSMTP(mailCtx, smtpCfg, mailer.Message{
		To:          recipients,
		Subject:     subject,
		TextBody:    body,
		Attachments: attachments,
	}); err != nil {
		a.logger.Error("send bug feedback mail failed", zap.Error(err))
		return err
	}
	return nil
}

func normalizeBugFeedbackRequest(request *types.SubmitBugFeedbackRequest) {
	request.Message = strings.TrimSpace(request.Message)
	request.ScreenshotDataURL = strings.TrimSpace(request.ScreenshotDataURL)
	request.ScreenshotName = strings.TrimSpace(request.ScreenshotName)
	request.PageURL = strings.TrimSpace(request.PageURL)
	request.Locale = strings.TrimSpace(request.Locale)
	request.UserAgent = strings.TrimSpace(request.UserAgent)
	request.ClientIP = strings.TrimSpace(request.ClientIP)
}

func validateBugFeedbackRequest(request *types.SubmitBugFeedbackRequest) error {
	messageLen := len([]rune(request.Message))
	if messageLen == 0 {
		return newBugFeedbackInvalid("反馈内容不能为空")
	}
	if messageLen < bugFeedbackMinMessageLength {
		return newBugFeedbackInvalid("反馈内容不能为空")
	}
	if messageLen > bugFeedbackMaxMessageLength {
		return newBugFeedbackInvalid("反馈内容不能超过2000个字符")
	}
	if len(request.ScreenshotDataURL) > bugFeedbackMaxScreenshotDataURL {
		return newBugFeedbackInvalid("截图不能超过2MB")
	}
	return nil
}

func (a *adminService) checkBugFeedbackLimit(ctx context.Context, clientIP string) error {
	cfg := a.bugFeedbackRateLimitConfig()
	if cfg.Disabled {
		return nil
	}
	ipHash := ratelimit.HashPart(normalizeClientIP(clientIP))
	key := cachekey.AdminRateLimit("bug-feedback", "ip", ipHash).String()
	err := a.limiter.Check(ctx, rateLimitRule(key, cfg.IP))
	if err == nil {
		return nil
	}
	if _, ok := ratelimit.AsLimited(err); ok {
		return err
	}
	return fmt.Errorf("bug feedback rate limit unavailable: %w", err)
}

func (a *adminService) bugFeedbackRateLimitConfig() appconfig.BugFeedbackRateLimitConfig {
	cfg := appconfig.BugFeedbackRateLimitConfig{}
	if a != nil && a.config != nil {
		cfg = a.config.RateLimitSnapshot().BugFeedback
	}
	fillWindowConfig(&cfg.IP, defaultBugFeedbackIPLimit, defaultBugFeedbackIPWindow)
	return cfg
}

func (a *adminService) bugFeedbackRecipients(ctx context.Context) ([]string, error) {
	aboutMe, err := a.UserGetAboutMe(ctx)
	if err != nil {
		return nil, err
	}
	recipients := make([]string, 0, len(aboutMe.Email))
	seen := make(map[string]struct{}, len(aboutMe.Email))
	for _, raw := range aboutMe.Email {
		addr, err := mail.ParseAddress(strings.TrimSpace(raw))
		if err != nil {
			continue
		}
		key := strings.ToLower(addr.Address)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		recipients = append(recipients, addr.Address)
	}
	if len(recipients) == 0 {
		return nil, ErrBugFeedbackRecipientNotConfigured
	}
	return recipients, nil
}

func (a *adminService) bugFeedbackSMTPConfig() (mailer.SMTPConfig, error) {
	cfg := appconfig.BugFeedbackSMTPConfig{}
	if a != nil && a.config != nil {
		cfg = a.config.BugFeedbackSnapshot().SMTP
	}
	password, err := utils.EnvOrFile(bugFeedbackSMTPPasswordEnv)
	if err != nil {
		return mailer.SMTPConfig{}, ErrBugFeedbackSMTPNotConfigured
	}
	host := firstNonEmptyEnv(bugFeedbackSMTPHostEnv, cfg.Host)
	username := firstNonEmptyEnv(bugFeedbackSMTPUsernameEnv, cfg.Username)
	from := firstNonEmptyEnv(bugFeedbackSMTPFromEnv, cfg.From)
	fromName := firstNonEmptyEnv(bugFeedbackSMTPFromNameEnv, cfg.FromName)

	port := cfg.Port
	if port <= 0 {
		port = defaultBugFeedbackSMTPPort
	}
	if rawPort := strings.TrimSpace(os.Getenv(bugFeedbackSMTPPortEnv)); rawPort != "" {
		parsed, err := strconv.Atoi(rawPort)
		if err != nil || parsed <= 0 || parsed > 65535 {
			return mailer.SMTPConfig{}, ErrBugFeedbackSMTPNotConfigured
		}
		port = parsed
	}
	if from == "" {
		from = username
	}
	if fromName == "" {
		fromName = defaultBugFeedbackSMTPFromName
	}
	if host == "" || username == "" || password == "" || from == "" {
		return mailer.SMTPConfig{}, ErrBugFeedbackSMTPNotConfigured
	}
	return mailer.SMTPConfig{
		Host:     host,
		Port:     port,
		Username: username,
		Password: password,
		From:     from,
		FromName: fromName,
	}, nil
}

func firstNonEmptyEnv(envKey, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(envKey)); value != "" {
		return value
	}
	return strings.TrimSpace(fallback)
}

func decodeBugFeedbackScreenshot(dataURL, rawName string) (*mailer.Attachment, error) {
	if dataURL == "" {
		return nil, nil
	}
	if len(dataURL) > bugFeedbackMaxScreenshotDataURL {
		return nil, newBugFeedbackInvalid("截图不能超过2MB")
	}
	if !strings.HasPrefix(dataURL, "data:") {
		return nil, newBugFeedbackInvalid("截图格式不支持")
	}
	comma := strings.IndexByte(dataURL, ',')
	if comma <= len("data:") {
		return nil, newBugFeedbackInvalid("截图格式不支持")
	}

	meta := strings.ToLower(dataURL[len("data:"):comma])
	if !strings.Contains(meta, ";base64") {
		return nil, newBugFeedbackInvalid("截图格式不支持")
	}
	contentType := strings.Split(meta, ";")[0]
	if !isAllowedBugFeedbackImageType(contentType) {
		return nil, newBugFeedbackInvalid("截图格式不支持")
	}

	data, err := base64.StdEncoding.DecodeString(dataURL[comma+1:])
	if err != nil {
		return nil, newBugFeedbackInvalid("截图格式不支持")
	}
	if len(data) > bugFeedbackMaxScreenshotBytes {
		return nil, newBugFeedbackInvalid("截图不能超过2MB")
	}

	return &mailer.Attachment{
		Filename:    sanitizeBugFeedbackScreenshotName(rawName, contentType),
		ContentType: contentType,
		Data:        data,
	}, nil
}

func isAllowedBugFeedbackImageType(contentType string) bool {
	switch contentType {
	case "image/png", "image/jpeg", "image/webp":
		return true
	default:
		return false
	}
}

func sanitizeBugFeedbackScreenshotName(rawName, contentType string) string {
	name := filepath.Base(strings.TrimSpace(rawName))
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.', r == '-', r == '_':
			b.WriteRune(r)
		}
		if b.Len() >= 80 {
			break
		}
	}
	name = strings.Trim(b.String(), ".-_")
	if name == "" {
		name = "bug-feedback-screenshot"
	}
	ext := strings.ToLower(filepath.Ext(name))
	expectedExt := extensionForBugFeedbackImageType(contentType)
	if expectedExt != "" && ext != expectedExt {
		name += expectedExt
	}
	return name
}

func extensionForBugFeedbackImageType(contentType string) string {
	switch contentType {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	default:
		extensions, err := mime.ExtensionsByType(contentType)
		if err == nil && len(extensions) > 0 {
			return extensions[0]
		}
		return ""
	}
}

func buildBugFeedbackMailBody(request *types.SubmitBugFeedbackRequest) string {
	var b strings.Builder
	b.WriteString("JSON Tool Bug Feedback\n\n")
	b.WriteString("Message:\n")
	b.WriteString(request.Message)
	b.WriteString("\n\nContext:\n")
	writeBugFeedbackLine(&b, "Page URL", request.PageURL)
	writeBugFeedbackLine(&b, "Locale", request.Locale)
	writeBugFeedbackLine(&b, "Client IP", request.ClientIP)
	writeBugFeedbackLine(&b, "User Agent", request.UserAgent)
	writeBugFeedbackLine(&b, "Submitted At", time.Now().Format(time.RFC3339))
	if request.ScreenshotDataURL != "" {
		writeBugFeedbackLine(&b, "Screenshot", "attached")
	} else {
		writeBugFeedbackLine(&b, "Screenshot", "none")
	}
	return b.String()
}

func writeBugFeedbackLine(b *strings.Builder, key, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = "-"
	}
	b.WriteString(key)
	b.WriteString(": ")
	b.WriteString(value)
	b.WriteByte('\n')
}

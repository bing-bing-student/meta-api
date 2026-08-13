package mailer

import (
	"fmt"
	"net/mail"
	"strings"
)

func validateConfig(cfg SMTPConfig) error {
	if strings.TrimSpace(cfg.Host) == "" {
		return fmt.Errorf("mailer: host is required")
	}
	if cfg.Port <= 0 {
		return fmt.Errorf("mailer: port is required")
	}
	if strings.TrimSpace(cfg.From) == "" {
		return fmt.Errorf("mailer: from is required")
	}
	if _, err := mail.ParseAddress(cfg.From); err != nil {
		return fmt.Errorf("mailer: invalid from address: %w", err)
	}
	return nil
}

func normalizeRecipients(raw []string) ([]string, error) {
	recipients := make([]string, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		addr, err := mail.ParseAddress(item)
		if err != nil {
			return nil, fmt.Errorf("mailer: invalid recipient address: %w", err)
		}
		normalized := strings.ToLower(addr.Address)
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		recipients = append(recipients, addr.Address)
	}
	if len(recipients) == 0 {
		return nil, fmt.Errorf("mailer: recipients are required")
	}
	return recipients, nil
}

func validateMessage(msg Message) error {
	if strings.TrimSpace(msg.Subject) == "" {
		return fmt.Errorf("mailer: subject is required")
	}
	return nil
}

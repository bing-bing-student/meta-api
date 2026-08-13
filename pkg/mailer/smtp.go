package mailer

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"time"
)

// SendSMTP sends a message through SMTP over TLS.
func SendSMTP(ctx context.Context, cfg SMTPConfig, msg Message) error {
	if err := validateConfig(cfg); err != nil {
		return err
	}
	recipients, err := normalizeRecipients(msg.To)
	if err != nil {
		return err
	}
	if err = validateMessage(msg); err != nil {
		return err
	}

	raw, err := buildMessage(cfg, recipients, msg)
	if err != nil {
		return err
	}

	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("mailer: dial smtp: %w", err)
	}
	defer conn.Close()

	tlsConn := tls.Client(conn, &tls.Config{
		ServerName: cfg.Host,
		MinVersion: tls.VersionTLS12,
	})
	if err = tlsConn.HandshakeContext(ctx); err != nil {
		return fmt.Errorf("mailer: tls handshake: %w", err)
	}
	if deadline, ok := ctx.Deadline(); ok {
		if err = tlsConn.SetDeadline(deadline); err != nil {
			return fmt.Errorf("mailer: set deadline: %w", err)
		}
	}

	client, err := smtp.NewClient(tlsConn, cfg.Host)
	if err != nil {
		return fmt.Errorf("mailer: new smtp client: %w", err)
	}
	defer client.Close()

	if err = client.Hello("localhost"); err != nil {
		return fmt.Errorf("mailer: hello: %w", err)
	}
	if cfg.Username != "" || cfg.Password != "" {
		auth := smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
		if err = client.Auth(auth); err != nil {
			return fmt.Errorf("mailer: auth: %w", err)
		}
	}
	if err = client.Mail(cfg.From); err != nil {
		return fmt.Errorf("mailer: mail from: %w", err)
	}
	for _, recipient := range recipients {
		if err = client.Rcpt(recipient); err != nil {
			return fmt.Errorf("mailer: rcpt %s: %w", recipient, err)
		}
	}

	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("mailer: data: %w", err)
	}
	if _, err = writer.Write(raw); err != nil {
		_ = writer.Close()
		return fmt.Errorf("mailer: write data: %w", err)
	}
	if err = writer.Close(); err != nil {
		return fmt.Errorf("mailer: close data: %w", err)
	}
	if err = client.Quit(); err != nil {
		return fmt.Errorf("mailer: quit: %w", err)
	}
	return nil
}

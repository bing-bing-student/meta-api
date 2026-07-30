package mailer

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strconv"
	"strings"
	"time"
)

// SMTPConfig describes the SMTP account used to send mail.
type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	FromName string
}

// Attachment is a single MIME attachment.
type Attachment struct {
	Filename    string
	ContentType string
	Data        []byte
}

// Message describes an email message.
type Message struct {
	To          []string
	Subject     string
	TextBody    string
	Attachments []Attachment
}

// SendSMTP sends a message through SMTP over TLS.
func SendSMTP(ctx context.Context, cfg SMTPConfig, msg Message) error {
	if err := validateConfig(cfg); err != nil {
		return err
	}
	recipients, err := normalizeRecipients(msg.To)
	if err != nil {
		return err
	}
	if strings.TrimSpace(msg.Subject) == "" {
		return fmt.Errorf("mailer: subject is required")
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

func buildMessage(cfg SMTPConfig, recipients []string, msg Message) ([]byte, error) {
	var buf bytes.Buffer
	from := mail.Address{Name: cfg.FromName, Address: cfg.From}
	writeHeader(&buf, "From", from.String())
	writeHeader(&buf, "To", strings.Join(recipients, ", "))
	writeHeader(&buf, "Subject", mime.QEncoding.Encode("UTF-8", msg.Subject))
	writeHeader(&buf, "Date", time.Now().Format(time.RFC1123Z))
	writeHeader(&buf, "MIME-Version", "1.0")

	if len(msg.Attachments) == 0 {
		writeHeader(&buf, "Content-Type", `text/plain; charset="UTF-8"`)
		writeHeader(&buf, "Content-Transfer-Encoding", "base64")
		buf.WriteString("\r\n")
		writeBase64(&buf, []byte(msg.TextBody))
		return buf.Bytes(), nil
	}

	mw := multipart.NewWriter(&buf)
	writeHeader(&buf, "Content-Type", `multipart/mixed; boundary="`+mw.Boundary()+`"`)
	buf.WriteString("\r\n")

	textHeader := textproto.MIMEHeader{}
	textHeader.Set("Content-Type", `text/plain; charset="UTF-8"`)
	textHeader.Set("Content-Transfer-Encoding", "base64")
	textPart, err := mw.CreatePart(textHeader)
	if err != nil {
		return nil, err
	}
	writeBase64(textPart, []byte(msg.TextBody))

	for _, attachment := range msg.Attachments {
		if len(attachment.Data) == 0 {
			continue
		}
		contentType := attachment.ContentType
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		filename := attachment.Filename
		if strings.TrimSpace(filename) == "" {
			filename = "attachment"
		}
		partHeader := textproto.MIMEHeader{}
		partHeader.Set("Content-Type", mime.FormatMediaType(contentType, map[string]string{"name": filename}))
		partHeader.Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
		partHeader.Set("Content-Transfer-Encoding", "base64")
		part, err := mw.CreatePart(partHeader)
		if err != nil {
			return nil, err
		}
		writeBase64(part, attachment.Data)
	}

	if err := mw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeHeader(buf *bytes.Buffer, key, value string) {
	buf.WriteString(key)
	buf.WriteString(": ")
	buf.WriteString(value)
	buf.WriteString("\r\n")
}

func writeBase64(w io.Writer, data []byte) {
	encoded := base64.StdEncoding.EncodeToString(data)
	for len(encoded) > 76 {
		_, _ = io.WriteString(w, encoded[:76]+"\r\n")
		encoded = encoded[76:]
	}
	if len(encoded) > 0 {
		_, _ = io.WriteString(w, encoded+"\r\n")
	}
}

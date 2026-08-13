package mailer

import (
	"bytes"
	"mime"
	"mime/multipart"
	"net/mail"
	"net/textproto"
	"strings"
	"time"
)

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

	if err := writeTextPart(mw, msg.TextBody); err != nil {
		return nil, err
	}
	if err := writeAttachmentParts(mw, msg.Attachments); err != nil {
		return nil, err
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeTextPart(mw *multipart.Writer, text string) error {
	textHeader := textproto.MIMEHeader{}
	textHeader.Set("Content-Type", `text/plain; charset="UTF-8"`)
	textHeader.Set("Content-Transfer-Encoding", "base64")
	textPart, err := mw.CreatePart(textHeader)
	if err != nil {
		return err
	}
	writeBase64(textPart, []byte(text))
	return nil
}

func writeAttachmentParts(mw *multipart.Writer, attachments []Attachment) error {
	for _, attachment := range attachments {
		if len(attachment.Data) == 0 {
			continue
		}
		part, err := mw.CreatePart(attachmentHeader(attachment))
		if err != nil {
			return err
		}
		writeBase64(part, attachment.Data)
	}
	return nil
}

func attachmentHeader(attachment Attachment) textproto.MIMEHeader {
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
	return partHeader
}

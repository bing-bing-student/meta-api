package mailer

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

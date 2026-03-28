package email

import (
	"fmt"
	"net/smtp"
	"strings"
)

// EmailService defines the interface for sending emails.
type EmailService interface {
	// Send sends a plain-text email to the specified recipient.
	Send(to, subject, body string) error

	// SendTemplate sends a templated email to the specified recipient.
	// The templateName identifies a pre-defined template, and data provides
	// the values to interpolate into the template.
	SendTemplate(to, subject, templateName string, data map[string]interface{}) error
}

// SMTPEmailService implements EmailService using net/smtp.
type SMTPEmailService struct {
	host     string
	port     string
	user     string
	password string
	from     string
}

// NewSMTPEmailService creates a new SMTPEmailService with the given SMTP configuration.
func NewSMTPEmailService(host, port, user, password, from string) *SMTPEmailService {
	return &SMTPEmailService{
		host:     host,
		port:     port,
		user:     user,
		password: password,
		from:     from,
	}
}

// Send sends a plain-text email via SMTP.
func (s *SMTPEmailService) Send(to, subject, body string) error {
	addr := fmt.Sprintf("%s:%s", s.host, s.port)
	auth := smtp.PlainAuth("", s.user, s.password, s.host)

	msg := buildMessage(s.from, to, subject, body)

	err := smtp.SendMail(addr, auth, s.from, []string{to}, []byte(msg))
	if err != nil {
		return fmt.Errorf("failed to send email to %s: %w", to, err)
	}
	return nil
}

// SendTemplate sends a templated email via SMTP.
// TODO: Implement template loading and rendering from a template store.
// Currently falls back to a simple key-value replacement in the body.
func (s *SMTPEmailService) SendTemplate(to, subject, templateName string, data map[string]interface{}) error {
	// TODO: Load template by templateName from a template registry or filesystem.
	// For now, construct a basic body from the data map.
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("Template: %s\n\n", templateName))
	for k, v := range data {
		builder.WriteString(fmt.Sprintf("%s: %v\n", k, v))
	}

	return s.Send(to, subject, builder.String())
}

// buildMessage constructs an RFC 2822 formatted email message.
func buildMessage(from, to, subject, body string) string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("From: %s\r\n", from))
	builder.WriteString(fmt.Sprintf("To: %s\r\n", to))
	builder.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	builder.WriteString("MIME-Version: 1.0\r\n")
	builder.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
	builder.WriteString("\r\n")
	builder.WriteString(body)
	return builder.String()
}

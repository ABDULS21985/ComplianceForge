package notification_channels

import (
	"context"
	"fmt"
	"net/smtp"

	"github.com/rs/zerolog/log"
)

// EmailChannel sends notifications via SMTP email.
type EmailChannel struct {
	Host     string
	Port     int
	User     string
	Password string
	From     string
}

// NewEmailChannel creates a new email notification channel.
func NewEmailChannel(host string, port int, user, password, from string) *EmailChannel {
	return &EmailChannel{Host: host, Port: port, User: user, Password: password, From: from}
}

// Send delivers an email notification.
func (ch *EmailChannel) Send(ctx context.Context, to, subject, bodyHTML, bodyText string) error {
	addr := fmt.Sprintf("%s:%d", ch.Host, ch.Port)

	headers := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n",
		ch.From, to, subject)
	msg := []byte(headers + bodyHTML)

	var auth smtp.Auth
	if ch.User != "" {
		auth = smtp.PlainAuth("", ch.User, ch.Password, ch.Host)
	}

	if err := smtp.SendMail(addr, auth, ch.From, []string{to}, msg); err != nil {
		log.Error().Err(err).Str("to", to).Str("subject", subject).Msg("email_channel: failed to send")
		return fmt.Errorf("sending email to %s: %w", to, err)
	}

	log.Info().Str("to", to).Str("subject", subject).Msg("email_channel: sent")
	return nil
}

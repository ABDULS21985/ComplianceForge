// Package email provides validated, context-aware SMTP delivery.
package email

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"sort"
	"strings"
	"time"
)

const (
	// TLSModeStartTLS requires the server to advertise STARTTLS before any
	// credentials or message content are transmitted.
	TLSModeStartTLS = "starttls"
	// TLSModeImplicit establishes TLS before the SMTP handshake (usually 465).
	TLSModeImplicit = "implicit"
	// TLSModeDisabled is intended only for local, unauthenticated test servers.
	TLSModeDisabled = "disabled"

	defaultTimeout = 15 * time.Second
	maxRecipients  = 100
	maxMessageSize = 10 << 20
)

var (
	ErrInvalidConfig  = errors.New("invalid SMTP configuration")
	ErrInvalidMessage = errors.New("invalid email message")
)

// Config describes one SMTP transport. TLS certificate verification is always
// enabled; deliberately insecure TLS options are not exposed.
type Config struct {
	Host       string
	Port       int
	Username   string
	Password   string
	From       string
	TLSMode    string
	Timeout    time.Duration
	HelloName  string
	ServerName string
}

// Message is an RFC 5322 email. Bcc recipients are included in the SMTP
// envelope but deliberately omitted from the rendered headers.
type Message struct {
	To       []string
	Cc       []string
	Bcc      []string
	ReplyTo  string
	Subject  string
	TextBody string
	HTMLBody string
	Headers  map[string]string
}

// Sender is the delivery boundary used by application services and tests.
type Sender interface {
	Send(context.Context, Message) error
}

// SMTPEmailService delivers messages through one validated SMTP transport.
type SMTPEmailService struct {
	config Config
	dialer net.Dialer
	now    func() time.Time
}

// NewSMTPEmailService validates configuration before constructing a sender.
func NewSMTPEmailService(cfg Config) (*SMTPEmailService, error) {
	cfg.Host = strings.TrimSpace(cfg.Host)
	cfg.From = strings.TrimSpace(cfg.From)
	cfg.Username = strings.TrimSpace(cfg.Username)
	cfg.TLSMode = strings.ToLower(strings.TrimSpace(cfg.TLSMode))
	cfg.HelloName = strings.TrimSpace(cfg.HelloName)
	cfg.ServerName = strings.TrimSpace(cfg.ServerName)
	if cfg.Timeout == 0 {
		cfg.Timeout = defaultTimeout
	}
	if cfg.ServerName == "" {
		cfg.ServerName = cfg.Host
	}

	if cfg.Host == "" || strings.ContainsAny(cfg.Host, "/\r\n") || cfg.Port < 1 || cfg.Port > 65535 {
		return nil, fmt.Errorf("%w: host and a valid port are required", ErrInvalidConfig)
	}
	if cfg.Timeout <= 0 || cfg.Timeout > 2*time.Minute {
		return nil, fmt.Errorf("%w: timeout must be between zero and two minutes", ErrInvalidConfig)
	}
	if cfg.TLSMode != TLSModeStartTLS && cfg.TLSMode != TLSModeImplicit && cfg.TLSMode != TLSModeDisabled {
		return nil, fmt.Errorf("%w: TLS mode must be starttls, implicit, or disabled", ErrInvalidConfig)
	}
	if (cfg.Username == "") != (cfg.Password == "") {
		return nil, fmt.Errorf("%w: username and password must be configured together", ErrInvalidConfig)
	}
	if cfg.Username != "" && cfg.TLSMode == TLSModeDisabled {
		return nil, fmt.Errorf("%w: authentication requires TLS", ErrInvalidConfig)
	}
	if _, err := parseSingleAddress(cfg.From); err != nil {
		return nil, fmt.Errorf("%w: invalid from address", ErrInvalidConfig)
	}
	if cfg.HelloName != "" && (hasHeaderInjection(cfg.HelloName) || strings.ContainsAny(cfg.HelloName, " /")) {
		return nil, fmt.Errorf("%w: invalid SMTP hello name", ErrInvalidConfig)
	}

	return &SMTPEmailService{
		config: cfg,
		dialer: net.Dialer{Timeout: cfg.Timeout, KeepAlive: -1},
		now:    time.Now,
	}, nil
}

// Send validates, renders, and synchronously delivers a message. Cancellation
// closes the underlying connection so callers are never stuck behind SMTP I/O.
func (s *SMTPEmailService) Send(ctx context.Context, message Message) error {
	if ctx == nil {
		return fmt.Errorf("send email: nil context")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("send email: %w", err)
	}

	from, recipients, err := validateMessage(s.config.From, message)
	if err != nil {
		return err
	}
	payload, err := buildMessage(from, message, s.now().UTC())
	if err != nil {
		return fmt.Errorf("render email: %w", err)
	}
	if len(payload) > maxMessageSize {
		return fmt.Errorf("%w: rendered message exceeds %d bytes", ErrInvalidMessage, maxMessageSize)
	}

	conn, err := s.dial(ctx)
	if err != nil {
		return fmt.Errorf("connect to SMTP server: %w", err)
	}
	defer conn.Close()

	deadline := s.now().Add(s.config.Timeout)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return fmt.Errorf("set SMTP deadline: %w", err)
	}

	cancelWatchDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-cancelWatchDone:
		}
	}()
	defer close(cancelWatchDone)

	client, err := smtp.NewClient(conn, s.config.Host)
	if err != nil {
		return smtpOperationError(ctx, "start SMTP session", err)
	}
	defer client.Close()

	if s.config.HelloName != "" {
		if err := client.Hello(s.config.HelloName); err != nil {
			return smtpOperationError(ctx, "send SMTP hello", err)
		}
	}

	if s.config.TLSMode == TLSModeStartTLS {
		if supported, _ := client.Extension("STARTTLS"); !supported {
			return fmt.Errorf("SMTP server does not support required STARTTLS")
		}
		if err := client.StartTLS(s.tlsConfig()); err != nil {
			return smtpOperationError(ctx, "negotiate STARTTLS", err)
		}
	}

	if s.config.Username != "" {
		if supported, _ := client.Extension("AUTH"); !supported {
			return fmt.Errorf("SMTP server does not support required authentication")
		}
		auth := smtp.PlainAuth("", s.config.Username, s.config.Password, s.config.Host)
		if err := client.Auth(auth); err != nil {
			return smtpOperationError(ctx, "authenticate to SMTP server", err)
		}
	}

	if err := client.Mail(from.Address); err != nil {
		return smtpOperationError(ctx, "set SMTP sender", err)
	}
	for _, recipient := range recipients {
		if err := client.Rcpt(recipient.Address); err != nil {
			return smtpOperationError(ctx, "set SMTP recipient", err)
		}
	}

	writer, err := client.Data()
	if err != nil {
		return smtpOperationError(ctx, "begin SMTP data", err)
	}
	if _, err := writer.Write(payload); err != nil {
		_ = writer.Close()
		return smtpOperationError(ctx, "write SMTP data", err)
	}
	if err := writer.Close(); err != nil {
		return smtpOperationError(ctx, "commit SMTP data", err)
	}
	if err := client.Quit(); err != nil {
		return smtpOperationError(ctx, "close SMTP session", err)
	}
	return nil
}

func (s *SMTPEmailService) dial(ctx context.Context) (net.Conn, error) {
	address := net.JoinHostPort(s.config.Host, fmt.Sprintf("%d", s.config.Port))
	if s.config.TLSMode == TLSModeImplicit {
		tlsDialer := tls.Dialer{NetDialer: &s.dialer, Config: s.tlsConfig()}
		return tlsDialer.DialContext(ctx, "tcp", address)
	}
	return s.dialer.DialContext(ctx, "tcp", address)
}

func (s *SMTPEmailService) tlsConfig() *tls.Config {
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		ServerName: s.config.ServerName,
	}
}

func smtpOperationError(ctx context.Context, operation string, err error) error {
	if contextErr := ctx.Err(); contextErr != nil {
		return fmt.Errorf("%s: %w", operation, contextErr)
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func validateMessage(configuredFrom string, message Message) (*mail.Address, []*mail.Address, error) {
	from, err := parseSingleAddress(configuredFrom)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: invalid sender", ErrInvalidMessage)
	}
	if message.Subject == "" || len(message.Subject) > 998 || hasHeaderInjection(message.Subject) {
		return nil, nil, fmt.Errorf("%w: subject is required and must not contain line breaks", ErrInvalidMessage)
	}
	if message.TextBody == "" && message.HTMLBody == "" {
		return nil, nil, fmt.Errorf("%w: a text or HTML body is required", ErrInvalidMessage)
	}
	if len(message.TextBody)+len(message.HTMLBody) > maxMessageSize {
		return nil, nil, fmt.Errorf("%w: message body exceeds %d bytes", ErrInvalidMessage, maxMessageSize)
	}
	if message.ReplyTo != "" {
		if _, err := parseSingleAddress(message.ReplyTo); err != nil {
			return nil, nil, fmt.Errorf("%w: invalid reply-to address", ErrInvalidMessage)
		}
	}
	for name, value := range message.Headers {
		canonicalName := textproto.CanonicalMIMEHeaderKey(strings.TrimSpace(name))
		if canonicalName == "" || !strings.HasPrefix(canonicalName, "X-") || hasHeaderInjection(name) || hasHeaderInjection(value) {
			return nil, nil, fmt.Errorf("%w: only safe X-* headers are accepted", ErrInvalidMessage)
		}
	}

	all := make([]string, 0, len(message.To)+len(message.Cc)+len(message.Bcc))
	all = append(all, message.To...)
	all = append(all, message.Cc...)
	all = append(all, message.Bcc...)
	if len(all) == 0 || len(all) > maxRecipients {
		return nil, nil, fmt.Errorf("%w: between 1 and %d recipients are required", ErrInvalidMessage, maxRecipients)
	}

	recipients := make([]*mail.Address, 0, len(all))
	seen := make(map[string]struct{}, len(all))
	for _, rawAddress := range all {
		address, err := parseSingleAddress(rawAddress)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: invalid recipient address", ErrInvalidMessage)
		}
		key := strings.ToLower(address.Address)
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		recipients = append(recipients, address)
	}
	return from, recipients, nil
}

func parseSingleAddress(value string) (*mail.Address, error) {
	if value == "" || hasHeaderInjection(value) {
		return nil, fmt.Errorf("invalid address")
	}
	address, err := mail.ParseAddress(value)
	if err != nil || address.Address == "" {
		return nil, fmt.Errorf("invalid address")
	}
	if strings.ContainsAny(address.Address, "\r\n") {
		return nil, fmt.Errorf("invalid address")
	}
	return address, nil
}

func buildMessage(from *mail.Address, message Message, sentAt time.Time) ([]byte, error) {
	var output bytes.Buffer
	writeHeader := func(name, value string) {
		fmt.Fprintf(&output, "%s: %s\r\n", name, value)
	}

	writeHeader("From", from.String())
	writeHeader("To", joinAddresses(message.To))
	if len(message.Cc) > 0 {
		writeHeader("Cc", joinAddresses(message.Cc))
	}
	if message.ReplyTo != "" {
		replyTo, _ := parseSingleAddress(message.ReplyTo)
		writeHeader("Reply-To", replyTo.String())
	}
	writeHeader("Subject", mime.QEncoding.Encode("utf-8", message.Subject))
	writeHeader("Date", sentAt.Format(time.RFC1123Z))
	writeHeader("Message-ID", newMessageID(from.Address, sentAt))
	writeHeader("MIME-Version", "1.0")

	type header struct{ name, value string }
	headers := make([]header, 0, len(message.Headers))
	for name, value := range message.Headers {
		headers = append(headers, header{
			name:  textproto.CanonicalMIMEHeaderKey(strings.TrimSpace(name)),
			value: value,
		})
	}
	sort.Slice(headers, func(i, j int) bool { return headers[i].name < headers[j].name })
	for _, item := range headers {
		writeHeader(item.name, item.value)
	}

	if message.TextBody != "" && message.HTMLBody != "" {
		multipartWriter := multipart.NewWriter(&output)
		writeHeader("Content-Type", fmt.Sprintf("multipart/alternative; boundary=%q", multipartWriter.Boundary()))
		output.WriteString("\r\n")
		if err := writeMIMEPart(multipartWriter, "text/plain; charset=utf-8", message.TextBody); err != nil {
			return nil, err
		}
		if err := writeMIMEPart(multipartWriter, "text/html; charset=utf-8", message.HTMLBody); err != nil {
			return nil, err
		}
		if err := multipartWriter.Close(); err != nil {
			return nil, err
		}
		return output.Bytes(), nil
	}

	contentType, body := "text/plain; charset=utf-8", message.TextBody
	if body == "" {
		contentType, body = "text/html; charset=utf-8", message.HTMLBody
	}
	writeHeader("Content-Type", contentType)
	writeHeader("Content-Transfer-Encoding", "quoted-printable")
	output.WriteString("\r\n")
	encoded := quotedprintable.NewWriter(&output)
	if _, err := io.WriteString(encoded, normalizeBody(body)); err != nil {
		return nil, err
	}
	if err := encoded.Close(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func writeMIMEPart(writer *multipart.Writer, contentType, body string) error {
	header := make(textproto.MIMEHeader)
	header.Set("Content-Type", contentType)
	header.Set("Content-Transfer-Encoding", "quoted-printable")
	part, err := writer.CreatePart(header)
	if err != nil {
		return err
	}
	encoded := quotedprintable.NewWriter(part)
	if _, err := io.WriteString(encoded, normalizeBody(body)); err != nil {
		return err
	}
	return encoded.Close()
}

func joinAddresses(values []string) string {
	formatted := make([]string, 0, len(values))
	for _, value := range values {
		address, err := parseSingleAddress(value)
		if err == nil {
			formatted = append(formatted, address.String())
		}
	}
	return strings.Join(formatted, ", ")
}

func newMessageID(from string, sentAt time.Time) string {
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		// The timestamp remains unique enough for the extremely unlikely case in
		// which the OS CSPRNG is unavailable; it contains no user data.
		return fmt.Sprintf("<%d@%s>", sentAt.UnixNano(), addressDomain(from))
	}
	return fmt.Sprintf("<%x@%s>", random, addressDomain(from))
}

func addressDomain(address string) string {
	if index := strings.LastIndexByte(address, '@'); index >= 0 && index+1 < len(address) {
		return address[index+1:]
	}
	return "localhost"
}

func normalizeBody(body string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\r", "\n")
	return strings.ReplaceAll(body, "\n", "\r\n")
}

func hasHeaderInjection(value string) bool {
	return strings.ContainsAny(value, "\r\n")
}

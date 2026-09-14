package email

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/textproto"
	"strings"
	"testing"
	"time"
)

func TestNewSMTPEmailServiceRejectsUnsafeConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{name: "missing host", mutate: func(c *Config) { c.Host = "" }},
		{name: "bad port", mutate: func(c *Config) { c.Port = 0 }},
		{name: "unknown TLS mode", mutate: func(c *Config) { c.TLSMode = "opportunistic" }},
		{name: "partial credentials", mutate: func(c *Config) { c.Username = "mailer" }},
		{name: "plaintext credentials", mutate: func(c *Config) { c.Username, c.Password = "mailer", "secret" }},
		{name: "header injection", mutate: func(c *Config) { c.From = "safe@example.com\r\nBcc: stolen@example.com" }},
		{name: "unbounded timeout", mutate: func(c *Config) { c.Timeout = 3 * time.Minute }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := testConfig()
			test.mutate(&cfg)
			_, err := NewSMTPEmailService(cfg)
			if !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("NewSMTPEmailService() error = %v, want ErrInvalidConfig", err)
			}
		})
	}
}

func TestSendRejectsHeaderInjectionBeforeDial(t *testing.T) {
	sender, err := NewSMTPEmailService(testConfig())
	if err != nil {
		t.Fatal(err)
	}

	err = sender.Send(context.Background(), Message{
		To:       []string{"recipient@example.com"},
		Subject:  "Quarterly report\r\nBcc: stolen@example.com",
		TextBody: "attached",
	})
	if !errors.Is(err, ErrInvalidMessage) {
		t.Fatalf("Send() error = %v, want ErrInvalidMessage", err)
	}
}

func TestBuildMessageCreatesAlternativeMIMEAndOmitsBcc(t *testing.T) {
	from, err := mail.ParseAddress("ComplianceForge <noreply@example.com>")
	if err != nil {
		t.Fatal(err)
	}
	payload, err := buildMessage(from, Message{
		To:       []string{"Primary <primary@example.com>"},
		Cc:       []string{"copy@example.com"},
		Bcc:      []string{"hidden@example.com"},
		Subject:  "Résumé ready",
		TextBody: "Line one\nLine two",
		HTMLBody: "<p>Line one</p>",
		Headers:  map[string]string{"X-Correlation-ID": "request-123"},
	}, time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	message := string(payload)
	for _, expected := range []string{
		"multipart/alternative",
		"Content-Type: text/plain; charset=utf-8",
		"Content-Type: text/html; charset=utf-8",
		"X-Correlation-Id: request-123",
		"=?utf-8?q?R=C3=A9sum=C3=A9_ready?=",
	} {
		if !strings.Contains(message, expected) {
			t.Errorf("message missing %q:\n%s", expected, message)
		}
	}
	if strings.Contains(message, "hidden@example.com") || strings.Contains(strings.ToLower(message), "bcc:") {
		t.Fatalf("Bcc recipient leaked into headers:\n%s", message)
	}
}

func TestSMTPEmailServiceDeliversEnvelopeAndMessage(t *testing.T) {
	server := startTestSMTPServer(t, false)
	cfg := testConfig()
	cfg.Host, cfg.Port = server.host, server.port
	sender, err := NewSMTPEmailService(cfg)
	if err != nil {
		t.Fatal(err)
	}
	sender.now = func() time.Time { return time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC) }

	err = sender.Send(context.Background(), Message{
		To:       []string{"primary@example.com"},
		Cc:       []string{"copy@example.com"},
		Bcc:      []string{"hidden@example.com"},
		Subject:  "Delivery test",
		TextBody: "hello",
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	delivery := <-server.deliveries
	if got, want := len(delivery.recipients), 3; got != want {
		t.Fatalf("recipients = %v, want %d", delivery.recipients, want)
	}
	if !strings.Contains(delivery.payload, "Subject: Delivery test") {
		t.Fatalf("payload missing subject:\n%s", delivery.payload)
	}
	if strings.Contains(delivery.payload, "hidden@example.com") {
		t.Fatal("Bcc address appeared in message headers")
	}
}

func TestSMTPEmailServiceRequiresAdvertisedSTARTTLS(t *testing.T) {
	server := startTestSMTPServer(t, false)
	cfg := testConfig()
	cfg.Host, cfg.Port, cfg.TLSMode = server.host, server.port, TLSModeStartTLS
	sender, err := NewSMTPEmailService(cfg)
	if err != nil {
		t.Fatal(err)
	}

	err = sender.Send(context.Background(), Message{
		To: []string{"recipient@example.com"}, Subject: "test", TextBody: "body",
	})
	if err == nil || !strings.Contains(err.Error(), "required STARTTLS") {
		t.Fatalf("Send() error = %v, want required STARTTLS error", err)
	}
}

func TestSMTPEmailServiceHonorsContextCancellation(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr == nil {
			defer connection.Close()
			_, _ = bufio.NewReader(connection).ReadString('\n')
		}
	}()
	host, port := splitListenerAddress(t, listener.Addr())
	cfg := testConfig()
	cfg.Host, cfg.Port, cfg.Timeout = host, port, time.Second
	sender, err := NewSMTPEmailService(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()

	err = sender.Send(ctx, Message{
		To: []string{"recipient@example.com"}, Subject: "test", TextBody: "body",
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Send() error = %v, want context deadline exceeded", err)
	}
}

func testConfig() Config {
	return Config{
		Host:    "127.0.0.1",
		Port:    1025,
		From:    "ComplianceForge <noreply@example.com>",
		TLSMode: TLSModeDisabled,
		Timeout: time.Second,
	}
}

type smtpDelivery struct {
	recipients []string
	payload    string
}

type testSMTPServer struct {
	host       string
	port       int
	deliveries chan smtpDelivery
}

func startTestSMTPServer(t *testing.T, advertiseSTARTTLS bool) *testSMTPServer {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	host, port := splitListenerAddress(t, listener.Addr())
	server := &testSMTPServer{host: host, port: port, deliveries: make(chan smtpDelivery, 1)}

	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer connection.Close()
		reader := textproto.NewReader(bufio.NewReader(connection))
		writer := bufio.NewWriter(connection)
		writeResponse := func(format string, args ...any) bool {
			_, writeErr := fmt.Fprintf(writer, format, args...)
			return writeErr == nil && writer.Flush() == nil
		}
		if !writeResponse("220 smtp.test ESMTP ready\r\n") {
			return
		}
		var recipients []string
		for {
			line, readErr := reader.ReadLine()
			if readErr != nil {
				return
			}
			command := strings.ToUpper(strings.Fields(line)[0])
			switch command {
			case "EHLO", "HELO":
				if advertiseSTARTTLS {
					if !writeResponse("250-smtp.test\r\n250 STARTTLS\r\n") {
						return
					}
				} else if !writeResponse("250 smtp.test\r\n") {
					return
				}
			case "MAIL":
				if !writeResponse("250 sender accepted\r\n") {
					return
				}
			case "RCPT":
				recipients = append(recipients, line)
				if !writeResponse("250 recipient accepted\r\n") {
					return
				}
			case "DATA":
				if !writeResponse("354 send message\r\n") {
					return
				}
				payload, dotErr := reader.ReadDotBytes()
				if dotErr != nil || !writeResponse("250 queued\r\n") {
					return
				}
				server.deliveries <- smtpDelivery{recipients: recipients, payload: string(payload)}
			case "QUIT":
				_ = writeResponse("221 closing\r\n")
				return
			default:
				if !writeResponse("502 unsupported\r\n") {
					return
				}
			}
		}
	}()
	return server
}

func splitListenerAddress(t *testing.T, address net.Addr) (string, int) {
	t.Helper()
	host, portText, err := net.SplitHostPort(address.String())
	if err != nil {
		t.Fatal(err)
	}
	var port int
	if _, err := fmt.Sscanf(portText, "%d", &port); err != nil {
		t.Fatal(err)
	}
	return host, port
}

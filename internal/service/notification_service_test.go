package service

import (
	"context"
	"errors"
	"testing"

	"github.com/rs/zerolog"

	emailpkg "github.com/complianceforge/platform/internal/pkg/email"
)

type recordingEmailSender struct {
	messages []emailpkg.Message
	err      error
}

func (s *recordingEmailSender) Send(_ context.Context, message emailpkg.Message) error {
	s.messages = append(s.messages, message)
	return s.err
}

func TestNotificationServiceForwardsHTMLMessage(t *testing.T) {
	sender := &recordingEmailSender{}
	service, err := NewNotificationServiceWithSender(sender, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}

	err = service.SendEmail(context.Background(), EmailMessage{
		To:      []string{"owner@example.com"},
		Cc:      []string{"auditor@example.com"},
		ReplyTo: "support@example.com",
		Subject: "Control changed",
		Body:    "<p>Review required</p>",
		IsHTML:  true,
	})
	if err != nil {
		t.Fatalf("SendEmail() error = %v", err)
	}
	if len(sender.messages) != 1 {
		t.Fatalf("sent messages = %d, want 1", len(sender.messages))
	}
	message := sender.messages[0]
	if message.HTMLBody != "<p>Review required</p>" || message.TextBody != "" {
		t.Fatalf("forwarded body = %#v", message)
	}
	if message.ReplyTo != "support@example.com" || len(message.Cc) != 1 {
		t.Fatalf("forwarded recipients = %#v", message)
	}
}

func TestNotificationServicePropagatesDeliveryFailure(t *testing.T) {
	deliveryErr := errors.New("provider unavailable")
	sender := &recordingEmailSender{err: deliveryErr}
	service, err := NewNotificationServiceWithSender(sender, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}

	err = service.NotifyRiskEscalation(
		context.Background(), "org-1", "Critical dependency", "medium", "critical",
		[]string{"owner@example.com"},
	)
	if !errors.Is(err, deliveryErr) {
		t.Fatalf("NotifyRiskEscalation() error = %v, want delivery failure", err)
	}
}

func TestNotificationServiceRequiresSender(t *testing.T) {
	if _, err := NewNotificationServiceWithSender(nil, zerolog.Nop()); err == nil {
		t.Fatal("NewNotificationServiceWithSender() accepted nil sender")
	}
}

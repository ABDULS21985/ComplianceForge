package service

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"

	"github.com/complianceforge/platform/internal/config"
	emailpkg "github.com/complianceforge/platform/internal/pkg/email"
)

// NotificationType categorizes the kind of notification being sent.
type NotificationType string

const (
	NotificationTypeEmail   NotificationType = "email"
	NotificationTypeWebhook NotificationType = "webhook"
	NotificationTypeSlack   NotificationType = "slack"
)

// EmailMessage holds the data for an outgoing email notification.
type EmailMessage struct {
	To      []string `json:"to"`
	Cc      []string `json:"cc,omitempty"`
	Bcc     []string `json:"bcc,omitempty"`
	ReplyTo string   `json:"reply_to,omitempty"`
	Subject string   `json:"subject"`
	Body    string   `json:"body"`
	IsHTML  bool     `json:"is_html"`
}

// NotificationService sends application notifications through injected,
// independently testable delivery transports.
type NotificationService struct {
	emailSender emailpkg.Sender
	logger      zerolog.Logger
}

// NewNotificationService validates SMTP configuration at startup. Invalid or
// incomplete delivery configuration is never silently replaced by logging.
func NewNotificationService(smtpCfg config.SMTPConfig, logger zerolog.Logger) (*NotificationService, error) {
	sender, err := emailpkg.NewSMTPEmailService(emailpkg.Config{
		Host:       smtpCfg.Host,
		Port:       smtpCfg.Port,
		Username:   smtpCfg.User,
		Password:   smtpCfg.Password,
		From:       smtpCfg.From,
		TLSMode:    smtpCfg.TLSMode,
		Timeout:    time.Duration(smtpCfg.TimeoutSeconds) * time.Second,
		HelloName:  smtpCfg.HelloName,
		ServerName: smtpCfg.ServerName,
	})
	if err != nil {
		return nil, fmt.Errorf("initialize notification email transport: %w", err)
	}
	return NewNotificationServiceWithSender(sender, logger)
}

// NewNotificationServiceWithSender constructs a service with an explicit
// sender. It is useful for provider adapters and deterministic tests.
func NewNotificationServiceWithSender(sender emailpkg.Sender, logger zerolog.Logger) (*NotificationService, error) {
	if sender == nil {
		return nil, fmt.Errorf("notification email sender is required")
	}
	return &NotificationService{
		emailSender: sender,
		logger:      logger.With().Str("service", "notification").Logger(),
	}, nil
}

// SendEmail synchronously hands a validated message to the configured sender.
func (s *NotificationService) SendEmail(ctx context.Context, msg EmailMessage) error {
	message := emailpkg.Message{
		To:      msg.To,
		Cc:      msg.Cc,
		Bcc:     msg.Bcc,
		ReplyTo: msg.ReplyTo,
		Subject: msg.Subject,
	}
	if msg.IsHTML {
		message.HTMLBody = msg.Body
	} else {
		message.TextBody = msg.Body
	}
	if err := s.emailSender.Send(ctx, message); err != nil {
		return fmt.Errorf("send email notification: %w", err)
	}
	s.logger.Info().
		Int("recipient_count", len(msg.To)+len(msg.Cc)+len(msg.Bcc)).
		Bool("is_html", msg.IsHTML).
		Msg("email notification delivered to SMTP server")
	return nil
}

// NotifyComplianceChange sends a notification when a control's compliance
// status changes significantly.
func (s *NotificationService) NotifyComplianceChange(ctx context.Context, orgID, frameworkName, controlCode, oldStatus, newStatus string, recipients []string) error {
	subject := fmt.Sprintf("[ComplianceForge] Compliance Status Change: %s - %s", frameworkName, controlCode)
	body := fmt.Sprintf(
		"Control %s in framework %s has changed status from %s to %s.\n\nPlease review this change in the ComplianceForge platform.",
		controlCode, frameworkName, oldStatus, newStatus,
	)

	msg := EmailMessage{
		To:      recipients,
		Subject: subject,
		Body:    body,
		IsHTML:  false,
	}

	if err := s.SendEmail(ctx, msg); err != nil {
		s.logger.Error().Err(err).
			Str("org_id", orgID).
			Str("control_code", controlCode).
			Msg("failed to send compliance change notification")
		return err
	}

	return nil
}

// NotifyRiskEscalation sends a notification when a risk is escalated to a
// higher severity level.
func (s *NotificationService) NotifyRiskEscalation(ctx context.Context, orgID, riskTitle, oldLevel, newLevel string, recipients []string) error {
	subject := fmt.Sprintf("[ComplianceForge] Risk Escalated: %s", riskTitle)
	body := fmt.Sprintf(
		"Risk '%s' has been escalated from %s to %s.\n\nImmediate attention may be required. Please review in the ComplianceForge platform.",
		riskTitle, oldLevel, newLevel,
	)

	msg := EmailMessage{
		To:      recipients,
		Subject: subject,
		Body:    body,
		IsHTML:  false,
	}

	if err := s.SendEmail(ctx, msg); err != nil {
		s.logger.Error().Err(err).
			Str("org_id", orgID).
			Str("risk_title", riskTitle).
			Msg("failed to send risk escalation notification")
		return err
	}

	return nil
}

// NotifyIncidentCreated sends a notification when a new security incident is reported.
func (s *NotificationService) NotifyIncidentCreated(ctx context.Context, orgID, incidentTitle, severity string, isBreachNotifiable bool, recipients []string) error {
	subject := fmt.Sprintf("[ComplianceForge] New Incident: %s [%s]", incidentTitle, severity)
	body := fmt.Sprintf(
		"A new security incident has been reported:\n\nTitle: %s\nSeverity: %s\n",
		incidentTitle, severity,
	)

	if isBreachNotifiable {
		body += "\nThis incident has been flagged as BREACH-NOTIFIABLE.\nGDPR Article 33 requires notification to the supervisory authority within 72 hours of detection.\n"
		subject = "[URGENT] " + subject
	}

	body += "\nPlease review and respond in the ComplianceForge platform."

	msg := EmailMessage{
		To:      recipients,
		Subject: subject,
		Body:    body,
		IsHTML:  false,
	}

	if err := s.SendEmail(ctx, msg); err != nil {
		s.logger.Error().Err(err).
			Str("org_id", orgID).
			Str("incident_title", incidentTitle).
			Msg("failed to send incident creation notification")
		return err
	}

	return nil
}

// NotifyAuditScheduled sends a notification when a new audit is scheduled.
func (s *NotificationService) NotifyAuditScheduled(ctx context.Context, orgID, auditTitle, auditType, startDate string, recipients []string) error {
	subject := fmt.Sprintf("[ComplianceForge] Audit Scheduled: %s", auditTitle)
	body := fmt.Sprintf(
		"A new audit has been scheduled:\n\nTitle: %s\nType: %s\nScheduled Start: %s\n\nPlease prepare the necessary documentation and resources.",
		auditTitle, auditType, startDate,
	)

	msg := EmailMessage{
		To:      recipients,
		Subject: subject,
		Body:    body,
		IsHTML:  false,
	}

	if err := s.SendEmail(ctx, msg); err != nil {
		s.logger.Error().Err(err).
			Str("org_id", orgID).
			Str("audit_title", auditTitle).
			Msg("failed to send audit scheduled notification")
		return err
	}

	return nil
}

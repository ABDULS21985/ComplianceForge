package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	htmltemplate "html/template"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
	"golang.org/x/net/html"

	"github.com/complianceforge/platform/internal/database"
	emailpkg "github.com/complianceforge/platform/internal/pkg/email"
	"github.com/complianceforge/platform/internal/pkg/safehttp"
	"github.com/complianceforge/platform/internal/pkg/secretbox"
	"github.com/complianceforge/platform/internal/repository"
)

// NotificationRule represents a rule from the database.
type NotificationRule struct {
	ID                     string                 `json:"id"`
	OrgID                  string                 `json:"organization_id"`
	Name                   string                 `json:"name"`
	EventType              string                 `json:"event_type"`
	SeverityFilter         []string               `json:"severity_filter"`
	Conditions             map[string]interface{} `json:"conditions"`
	ChannelIDs             []string               `json:"channel_ids"`
	RecipientType          string                 `json:"recipient_type"` // role, owner, user, dpo, ciso
	RecipientIDs           []string               `json:"recipient_ids"`
	TemplateID             string                 `json:"template_id"`
	IsActive               bool                   `json:"is_active"`
	CooldownMinutes        int                    `json:"cooldown_minutes"`
	EscalationAfterMinutes *int                   `json:"escalation_after_minutes,omitempty"`
	EscalationChannelIDs   []string               `json:"escalation_channel_ids,omitempty"`
}

// Notification represents a notification record.
type Notification struct {
	ID              string     `json:"id"`
	OrgID           string     `json:"organization_id"`
	EventType       string     `json:"event_type"`
	RecipientUserID string     `json:"recipient_user_id"`
	ChannelType     string     `json:"channel_type"`
	ChannelID       string     `json:"channel_id,omitempty"`
	Subject         string     `json:"subject"`
	Body            string     `json:"body"`
	TextBody        string     `json:"-"`
	HTMLBody        string     `json:"-"`
	Status          string     `json:"status"`
	CreatedAt       time.Time  `json:"created_at"`
	ReadAt          *time.Time `json:"read_at"`
	AcknowledgedAt  *time.Time `json:"acknowledged_at,omitempty"`
	DigestFrequency string     `json:"digest_frequency,omitempty"`
	ScheduledFor    time.Time  `json:"scheduled_for,omitempty"`
	RetryCount      int        `json:"retry_count,omitempty"`
	MaxRetries      int        `json:"max_retries,omitempty"`
	LeaseOwner      string     `json:"-"`
	LeaseToken      string     `json:"-"`
}

// NotificationEngine orchestrates the notification pipeline.
type NotificationEngine struct {
	pool       *pgxpool.Pool
	bus        *EventBus
	email      emailpkg.Sender
	secrets    *secretbox.Box
	httpClient notificationHTTPClient
	state      repository.NotificationStateRepository
	stopCh     chan struct{}
	startOnce  sync.Once
	stopOnce   sync.Once
}

type notificationHTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

// NewNotificationEngine creates a new NotificationEngine.
func NewNotificationEngine(pool *pgxpool.Pool, bus *EventBus, senders ...emailpkg.Sender) *NotificationEngine {
	var sender emailpkg.Sender
	if len(senders) > 0 {
		sender = senders[0]
	}
	return &NotificationEngine{
		pool:       pool,
		bus:        bus,
		email:      sender,
		httpClient: safehttp.NewClient(10*time.Second, safehttp.Policy{}),
		state:      repository.NewNotificationRepository(pool),
		stopCh:     make(chan struct{}),
	}
}

// NewNotificationEngineWithProtector enables encrypted notification-channel
// configuration. Production composition should always use this constructor.
func NewNotificationEngineWithProtector(
	pool *pgxpool.Pool,
	bus *EventBus,
	sender emailpkg.Sender,
	protector *secretbox.Box,
) *NotificationEngine {
	engine := NewNotificationEngine(pool, bus, sender)
	engine.secrets = protector
	return engine
}

// Ready reports whether the engine can persist and deliver every supported
// production channel without falling back to plaintext secret storage.
func (ne *NotificationEngine) Ready() bool {
	return ne != nil && ne.pool != nil && ne.email != nil && ne.secrets != nil && ne.httpClient != nil && ne.state != nil
}

// Start begins listening for events from the EventBus and processes them in a background goroutine.
func (ne *NotificationEngine) Start(ctx context.Context) {
	ne.startOnce.Do(func() {
		if ne.bus == nil {
			log.Error().Msg("notification engine cannot start without an event bus")
			return
		}
		eventCh := ne.bus.Subscribe("*")
		go func() {
			log.Info().Msg("notification engine started")
			for {
				select {
				case <-ne.stopCh:
					log.Info().Msg("notification engine stopped")
					return
				case <-ctx.Done():
					log.Info().Msg("notification engine context cancelled")
					return
				case event, ok := <-eventCh:
					if !ok {
						log.Info().Msg("notification engine event channel closed")
						return
					}
					if err := ne.ProcessEvent(ctx, event); err != nil {
						log.Error().Err(err).
							Str("event_type", event.Type).
							Str("entity_id", event.EntityID).
							Msg("failed to process notification event")
					}
				}
			}
		}()
	})
}

// Stop signals the notification engine to shut down.
func (ne *NotificationEngine) Stop() {
	ne.stopOnce.Do(func() { close(ne.stopCh) })
}

// ProcessEvent is the core notification pipeline. It queries matching rules, evaluates
// conditions, determines recipients, renders templates, and dispatches notifications.
func (ne *NotificationEngine) ProcessEvent(ctx context.Context, event Event) error {
	if ctx == nil {
		return fmt.Errorf("notification context is required")
	}
	if ne.pool == nil {
		return fmt.Errorf("notification database is not configured")
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	if event.Data == nil {
		event.Data = make(map[string]any)
	}
	if event.ID == "" {
		event.ID = notificationEventID(event)
	}
	if err := validateNotificationEvent(event); err != nil {
		return err
	}
	return database.WithTenantConnection(ctx, ne.pool, event.OrgID, func(tenantCtx context.Context) error {
		return ne.processTenantEvent(tenantCtx, event)
	})
}

func (ne *NotificationEngine) processTenantEvent(ctx context.Context, event Event) error {
	log.Info().
		Str("event_type", event.Type).
		Str("org_id", event.OrgID).
		Str("entity_ref", event.EntityRef).
		Msg("processing notification event")

	// 1. Query active notification_rules matching event type and organization.
	rules, err := ne.fetchMatchingRules(ctx, event)
	if err != nil {
		return fmt.Errorf("fetch matching rules: %w", err)
	}

	if len(rules) == 0 {
		log.Debug().
			Str("event_type", event.Type).
			Str("org_id", event.OrgID).
			Msg("no matching notification rules found")
		return nil
	}

	var processingErrors []error
	for _, rule := range rules {
		// 2. Check severity filter.
		if !ne.matchesSeverityFilter(rule, event) {
			log.Debug().
				Str("rule_id", rule.ID).
				Str("event_severity", event.Severity).
				Msg("event severity does not match rule filter, skipping")
			continue
		}

		// 2b. Evaluate conditions against event data.
		if !ne.evaluateConditions(rule.Conditions, event.Data) {
			log.Debug().
				Str("rule_id", rule.ID).
				Msg("rule conditions not met, skipping")
			continue
		}

		// 3. Check cooldown: skip if a notification was sent recently for this rule+entity.
		if rule.CooldownMinutes > 0 {
			cooledDown, err := ne.checkCooldown(ctx, rule.ID, event.EntityID, rule.CooldownMinutes)
			if err != nil {
				log.Error().Err(err).Str("rule_id", rule.ID).Msg("cooldown check failed")
				processingErrors = append(processingErrors, fmt.Errorf("rule %s cooldown: %w", rule.ID, err))
				continue
			}
			if cooledDown {
				log.Debug().Str("rule_id", rule.ID).Msg("rule in cooldown period, skipping")
				continue
			}
		}

		// 4. Determine recipients.
		recipientIDs, err := ne.DetermineRecipients(ctx, rule, event)
		if err != nil {
			log.Error().Err(err).Str("rule_id", rule.ID).Msg("failed to determine recipients")
			processingErrors = append(processingErrors, fmt.Errorf("rule %s recipients: %w", rule.ID, err))
			continue
		}

		if len(recipientIDs) == 0 {
			log.Debug().Str("rule_id", rule.ID).Msg("no recipients found for rule")
			continue
		}

		// 5. Load and render the notification template.
		content, err := ne.loadAndRenderTemplate(ctx, rule.TemplateID, event)
		if err != nil {
			log.Error().Err(err).Str("rule_id", rule.ID).Msg("failed to render template")
			processingErrors = append(processingErrors, fmt.Errorf("rule %s template: %w", rule.ID, err))
			continue
		}

		// Only critical regulatory/deadline events override opt-outs, digests and
		// quiet hours. Non-critical events continue to honor user preferences.
		bypassPreferences := event.Severity == "critical" && ne.isBypassEvent(event.Type)

		// 6-7. Resolve preferences and durably enqueue one idempotent delivery.
		// Delivery is deliberately separated from event processing: the durable
		// worker owns leases, retries and terminal failure state.
		for _, recipientID := range recipientIDs {
			for _, channelID := range rule.ChannelIDs {
				channelType, _, err := ne.getChannelConfig(ctx, event.OrgID, channelID)
				if err != nil {
					log.Error().Err(err).
						Str("channel_id", channelID).
						Msg("failed to load channel config")
					processingErrors = append(processingErrors, fmt.Errorf("channel %s: %w", channelID, err))
					continue
				}

				preference, err := ne.checkUserPreference(ctx, event.OrgID, recipientID, event.Type, channelType)
				if err != nil {
					log.Error().Err(err).
						Str("user_id", recipientID).
						Msg("failed to check notification preference")
					processingErrors = append(processingErrors, fmt.Errorf("recipient preference: %w", err))
					continue
				}
				if bypassPreferences {
					preference.Enabled = true
					preference.DigestFrequency = "immediate"
					preference.QuietHoursStart = nil
					preference.QuietHoursEnd = nil
				}
				if !preference.Enabled {
					log.Debug().
						Str("user_id", recipientID).
						Str("channel_type", channelType).
						Msg("user has disabled this notification channel, skipping")
					continue
				}

				createdAt := time.Now().UTC()
				scheduledFor := notificationScheduledFor(createdAt, preference)
				body := content.Text
				if channelType == "email" && content.HTML != "" {
					body = content.HTML
				}
				notification := Notification{
					OrgID:           event.OrgID,
					EventType:       event.Type,
					RecipientUserID: recipientID,
					ChannelType:     channelType,
					Subject:         content.Subject,
					Body:            body,
					TextBody:        content.Text,
					HTMLBody:        content.HTML,
					Status:          "pending",
					CreatedAt:       createdAt,
					ChannelID:       channelID,
					DigestFrequency: preference.DigestFrequency,
					ScheduledFor:    scheduledFor,
				}

				_, err = ne.createNotificationRecord(ctx, notification, rule.ID, channelID, event)
				if err != nil {
					log.Error().Err(err).Msg("failed to create notification record")
					processingErrors = append(processingErrors, fmt.Errorf("create notification: %w", err))
					continue
				}
			}
		}
	}

	return errors.Join(processingErrors...)
}

// RenderTemplate parses a Go text/template string and executes it with the provided data.
func (ne *NotificationEngine) RenderTemplate(tmplStr string, data map[string]interface{}) (string, error) {
	if len(tmplStr) > 256*1024 {
		return "", fmt.Errorf("template exceeds maximum size")
	}
	tmpl, err := template.New("notification").Option("missingkey=error").Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}
	if buf.Len() > 1024*1024 {
		return "", fmt.Errorf("rendered template exceeds maximum size")
	}
	return buf.String(), nil
}

// ValidateNotificationTemplateDefinition validates a persisted template before
// it can be selected by a delivery rule. Runtime rendering remains strict, but
// save-time validation prevents malformed or explicitly unsafe definitions
// from entering the notification pipeline.
func ValidateNotificationTemplateDefinition(subject, textBody, htmlBody string) error {
	if strings.TrimSpace(subject) == "" {
		return fmt.Errorf("subject_template is required")
	}
	if len(subject) > 256*1024 || len(textBody) > 256*1024 || len(htmlBody) > 256*1024 {
		return fmt.Errorf("each template field must not exceed 256 KiB")
	}
	if strings.ContainsAny(subject, "\r\n") {
		return fmt.Errorf("subject_template must not contain line breaks")
	}
	if strings.TrimSpace(textBody) == "" && strings.TrimSpace(htmlBody) == "" {
		return fmt.Errorf("body_text_template or body_html_template is required")
	}
	if _, err := template.New("subject").Option("missingkey=error").Parse(subject); err != nil {
		return fmt.Errorf("invalid subject_template: %w", err)
	}
	if textBody != "" {
		if _, err := template.New("text").Option("missingkey=error").Parse(textBody); err != nil {
			return fmt.Errorf("invalid body_text_template: %w", err)
		}
	}
	if htmlBody != "" {
		if _, err := htmltemplate.New("html").Option("missingkey=error").Parse(htmlBody); err != nil {
			return fmt.Errorf("invalid body_html_template: %w", err)
		}
		if err := validateRenderedHTML(htmlBody); err != nil {
			return fmt.Errorf("invalid body_html_template: %w", err)
		}
	}
	return nil
}

func renderHTMLTemplate(tmplStr string, data map[string]interface{}) (string, error) {
	if len(tmplStr) > 256*1024 {
		return "", fmt.Errorf("template exceeds maximum size")
	}
	tmpl, err := htmltemplate.New("notification").Option("missingkey=error").Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("parse HTML template: %w", err)
	}
	var output bytes.Buffer
	if err := tmpl.Execute(&output, data); err != nil {
		return "", fmt.Errorf("execute HTML template: %w", err)
	}
	if output.Len() > 1024*1024 {
		return "", fmt.Errorf("rendered template exceeds maximum size")
	}
	return output.String(), nil
}

func stripHTMLTags(value string) string {
	document, err := html.Parse(strings.NewReader(value))
	if err != nil {
		return "View this notification in an HTML-capable email client."
	}
	var output strings.Builder
	var visit func(*html.Node)
	visit = func(node *html.Node) {
		if node.Type == html.TextNode {
			output.WriteString(node.Data)
			output.WriteByte(' ')
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(document)
	return strings.Join(strings.Fields(output.String()), " ")
}

func validateRenderedHTML(value string) error {
	document, err := html.Parse(strings.NewReader(value))
	if err != nil {
		return fmt.Errorf("parse rendered HTML: %w", err)
	}
	forbiddenElements := map[string]struct{}{
		"base": {}, "button": {}, "embed": {}, "form": {}, "iframe": {},
		"input": {}, "link": {}, "math": {}, "meta": {}, "object": {},
		"script": {}, "svg": {},
	}
	var inspect func(*html.Node) error
	inspect = func(node *html.Node) error {
		if node.Type == html.ElementNode {
			if _, forbidden := forbiddenElements[strings.ToLower(node.Data)]; forbidden {
				return fmt.Errorf("rendered HTML contains forbidden element %q", node.Data)
			}
			for _, attribute := range node.Attr {
				name := strings.ToLower(attribute.Key)
				if strings.HasPrefix(name, "on") || name == "srcdoc" {
					return fmt.Errorf("rendered HTML contains forbidden attribute %q", attribute.Key)
				}
				if name == "href" || name == "src" || name == "action" {
					parsed, parseErr := url.Parse(strings.TrimSpace(attribute.Val))
					if parseErr != nil || (parsed.IsAbs() && parsed.Scheme != "https" && parsed.Scheme != "mailto" && parsed.Scheme != "cid") {
						return fmt.Errorf("rendered HTML contains an unsafe URL")
					}
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if err := inspect(child); err != nil {
				return err
			}
		}
		return nil
	}
	return inspect(document)
}

// DetermineRecipients resolves the list of user IDs that should receive the notification
// based on the rule's recipient_type setting.
func (ne *NotificationEngine) DetermineRecipients(ctx context.Context, rule NotificationRule, event Event) ([]string, error) {
	switch rule.RecipientType {
	case "user", "custom":
		return ne.filterTenantUsers(ctx, event.OrgID, rule.RecipientIDs)

	case "role":
		return ne.findUsersByRoleIDs(ctx, event.OrgID, rule.RecipientIDs)

	case "owner":
		// The entity owner is embedded in event data.
		if ownerID, ok := event.Data["owner_id"].(string); ok && ownerID != "" {
			return ne.filterTenantUsers(ctx, event.OrgID, []string{ownerID})
		}
		return nil, nil

	case "assignee":
		if assigneeID, ok := event.Data["assignee_id"].(string); ok && assigneeID != "" {
			return ne.filterTenantUsers(ctx, event.OrgID, []string{assigneeID})
		}
		return nil, nil

	case "dpo":
		return ne.findUsersByRoleSlugs(ctx, event.OrgID, []string{"dpo", "data_protection_officer"})

	case "ciso":
		return ne.findUsersByRoleSlugs(ctx, event.OrgID, []string{"ciso", "chief_information_security_officer"})

	default:
		return nil, fmt.Errorf("unknown recipient_type: %s", rule.RecipientType)
	}
}

// Dispatch sends a notification via the appropriate channel.
func (ne *NotificationEngine) Dispatch(ctx context.Context, notification Notification, channelConfig map[string]any) error {
	switch notification.ChannelType {
	case "email":
		return ne.dispatchEmail(ctx, notification, channelConfig)
	case "in_app":
		return ne.dispatchInApp(ctx, notification)
	case "webhook":
		return ne.dispatchWebhook(ctx, notification, channelConfig)
	case "slack":
		return ne.dispatchSlack(ctx, notification, channelConfig)
	default:
		return notificationDeliveryError("channel_unsupported", true,
			fmt.Errorf("unsupported channel type: %s", notification.ChannelType))
	}
}

// --- Internal helper methods ---

func (ne *NotificationEngine) fetchMatchingRules(ctx context.Context, event Event) ([]NotificationRule, error) {
	query := `
		SELECT id, organization_id, name, event_type, severity_filter, conditions,
		       channel_ids, recipient_type, recipient_ids, template_id, is_active, cooldown_minutes,
		       escalation_after_minutes, COALESCE(escalation_channel_ids, '{}'::uuid[])
		FROM notification_rules
		WHERE is_active = true
		  AND organization_id = $1
		  AND (event_type = $2 OR event_type = '*')
		ORDER BY created_at ASC`

	rows, err := database.QuerierFromContext(ctx, ne.pool).Query(ctx, query, event.OrgID, event.Type)
	if err != nil {
		return nil, fmt.Errorf("query notification rules: %w", err)
	}
	defer rows.Close()

	var rules []NotificationRule
	for rows.Next() {
		var rule NotificationRule
		var conditionsJSON []byte
		var templateID *string

		err := rows.Scan(
			&rule.ID, &rule.OrgID, &rule.Name, &rule.EventType,
			&rule.SeverityFilter, &conditionsJSON,
			&rule.ChannelIDs, &rule.RecipientType, &rule.RecipientIDs,
			&templateID, &rule.IsActive, &rule.CooldownMinutes,
			&rule.EscalationAfterMinutes, &rule.EscalationChannelIDs,
		)
		if err != nil {
			return nil, fmt.Errorf("scan notification rule: %w", err)
		}

		if conditionsJSON != nil {
			if err := json.Unmarshal(conditionsJSON, &rule.Conditions); err != nil {
				return nil, fmt.Errorf("decode conditions for notification rule %s: %w", rule.ID, err)
			}
		}
		if templateID != nil {
			rule.TemplateID = *templateID
		}

		rules = append(rules, rule)
	}

	return rules, rows.Err()
}

func (ne *NotificationEngine) matchesSeverityFilter(rule NotificationRule, event Event) bool {
	// If no severity filter is set, match all severities.
	if len(rule.SeverityFilter) == 0 {
		return true
	}
	for _, s := range rule.SeverityFilter {
		if s == event.Severity {
			return true
		}
	}
	return false
}

func (ne *NotificationEngine) evaluateConditions(conditions map[string]interface{}, data map[string]interface{}) bool {
	// If no conditions specified, the rule always matches.
	if len(conditions) == 0 {
		return true
	}

	// Simple key-value equality match: every condition key must match the corresponding data value.
	for key, expected := range conditions {
		actual, exists := data[key]
		if !exists {
			return false
		}

		// Handle type-flexible comparison via JSON serialization.
		expectedJSON, _ := json.Marshal(expected)
		actualJSON, _ := json.Marshal(actual)
		if string(expectedJSON) != string(actualJSON) {
			return false
		}
	}

	return true
}

func (ne *NotificationEngine) checkCooldown(ctx context.Context, ruleID, entityID string, cooldownMinutes int) (bool, error) {
	query := `
		SELECT EXISTS(
			SELECT 1 FROM notifications
			WHERE rule_id = $1
			  AND event_payload->>'entity_id' = $2
			  AND (status IN ('pending', 'sent', 'delivered') OR (status = 'failed' AND dead_at IS NULL))
			  AND created_at > NOW() - make_interval(mins => $3)
		)`

	var inCooldown bool
	err := database.QuerierFromContext(ctx, ne.pool).QueryRow(ctx, query, ruleID, entityID, cooldownMinutes).Scan(&inCooldown)
	if err != nil {
		return false, err
	}
	return inCooldown, nil
}

type renderedNotification struct {
	Subject string
	Text    string
	HTML    string
}

func (ne *NotificationEngine) loadAndRenderTemplate(ctx context.Context, templateID string, event Event) (renderedNotification, error) {
	var subjectTmpl, textTmpl, htmlTmpl string

	if templateID != "" {
		query := `
			SELECT COALESCE(subject_template, ''), COALESCE(body_text_template, ''), COALESCE(body_html_template, '')
			FROM notification_templates
			WHERE id = $1 AND (organization_id IS NULL OR organization_id = $2)`
		err := database.QuerierFromContext(ctx, ne.pool).QueryRow(ctx, query, templateID, event.OrgID).
			Scan(&subjectTmpl, &textTmpl, &htmlTmpl)
		if err != nil && err != pgx.ErrNoRows {
			return renderedNotification{}, fmt.Errorf("load template %s: %w", templateID, err)
		}
	}

	// Fallback to default templates if none found.
	if subjectTmpl == "" {
		subjectTmpl = "[ComplianceForge] {{.event_type}}: {{.entity_ref}}"
	}
	if textTmpl == "" && htmlTmpl == "" {
		textTmpl = "Event: {{.event_type}}\nEntity: {{.entity_ref}} ({{.entity_type}})\nSeverity: {{.severity}}\nTime: {{.timestamp}}"
	}

	// Merge event fields into the data map for template rendering.
	data := make(map[string]interface{})
	for k, v := range event.Data {
		data[k] = v
	}
	data["event_type"] = event.Type
	data["severity"] = event.Severity
	data["entity_type"] = event.EntityType
	data["entity_id"] = event.EntityID
	data["entity_ref"] = event.EntityRef
	data["org_id"] = event.OrgID
	data["timestamp"] = event.Timestamp.Format(time.RFC3339)

	subject, err := ne.RenderTemplate(subjectTmpl, data)
	if err != nil {
		return renderedNotification{}, fmt.Errorf("render subject template: %w", err)
	}
	subject = strings.TrimSpace(subject)
	if subject == "" || len(subject) > 255 || strings.ContainsAny(subject, "\r\n") {
		return renderedNotification{}, fmt.Errorf("rendered subject is invalid")
	}
	var textBody, htmlBody string
	if textTmpl != "" {
		textBody, err = ne.RenderTemplate(textTmpl, data)
		if err != nil {
			return renderedNotification{}, fmt.Errorf("render text body template: %w", err)
		}
	}
	if htmlTmpl != "" {
		htmlBody, err = renderHTMLTemplate(htmlTmpl, data)
		if err != nil {
			return renderedNotification{}, fmt.Errorf("render HTML body template: %w", err)
		}
		if err := validateRenderedHTML(htmlBody); err != nil {
			return renderedNotification{}, err
		}
	}
	if textBody == "" {
		textBody = stripHTMLTags(htmlBody)
	}
	return renderedNotification{Subject: subject, Text: textBody, HTML: htmlBody}, nil
}

func (ne *NotificationEngine) isBypassEvent(eventType string) bool {
	// Breach and regulatory events bypass user notification preferences.
	bypassTypes := map[string]bool{
		"incident.breach_deadline":        true,
		"incident.breach_created":         true,
		"gdpr.breach_72h_warning":         true,
		"gdpr.breach_deadline_imminent":   true,
		"gdpr.breach_deadline_exceeded":   true,
		"nis2.early_warning_deadline":     true,
		"nis2.full_report_deadline":       true,
		"nis2.deadline_exceeded":          true,
		"nis2.deadline_imminent":          true,
		"regulatory.deadline_approaching": true,
		"regulatory.deadline_exceeded":    true,
		"dsr.deadline_approaching":        true,
		"dsr.deadline_exceeded":           true,
		"dsr.deadline_imminent":           true,
	}
	return bypassTypes[eventType]
}

func (ne *NotificationEngine) getChannelConfig(ctx context.Context, orgID, channelID string) (string, map[string]any, error) {
	var channelType string
	var configJSON []byte

	query := `
		SELECT channel_type, configuration
		FROM notification_channels
		WHERE id = $1 AND organization_id = $2 AND is_active = true AND deleted_at IS NULL`
	err := database.QuerierFromContext(ctx, ne.pool).QueryRow(ctx, query, channelID, orgID).Scan(&channelType, &configJSON)
	if err != nil {
		return "", nil, fmt.Errorf("load channel %s: %w", channelID, err)
	}

	config, err := openNotificationChannelConfig(ne.secrets, orgID, channelID, channelType, configJSON)
	if err != nil {
		return "", nil, fmt.Errorf("decode channel %s configuration: %w", channelID, err)
	}

	return channelType, config, nil
}

func openNotificationChannelConfig(
	protector *secretbox.Box,
	orgID, channelID, channelType string,
	document []byte,
) (map[string]any, error) {
	config := make(map[string]any)
	if protector != nil {
		if err := protector.OpenJSON(orgID, notificationChannelSecretPurpose(channelID), document, &config); err == nil {
			return config, nil
		}
	}

	// Empty legacy configurations contain no secret material and can be read
	// during rolling upgrades. Sensitive legacy configurations fail closed and
	// must be recreated so their credentials are encrypted.
	if channelType == "email" || channelType == "in_app" {
		var legacy map[string]any
		if len(document) == 0 {
			return config, nil
		}
		if err := json.Unmarshal(document, &legacy); err == nil && len(legacy) == 0 {
			return config, nil
		}
	}
	return nil, fmt.Errorf("encrypted channel configuration is unavailable")
}

func notificationChannelSecretPurpose(channelID string) string {
	return "notification-channel/" + channelID
}

func (ne *NotificationEngine) checkUserPreference(ctx context.Context, orgID, userID, eventType, channelType string) (notificationPreferenceDecision, error) {
	var columnName string
	switch channelType {
	case "email":
		columnName = "email_enabled"
	case "in_app":
		columnName = "in_app_enabled"
	case "slack":
		columnName = "slack_enabled"
	case "webhook", "teams":
		// These are organization-owned delivery channels rather than personal
		// channels and therefore have no per-user preference column.
		return defaultNotificationPreference(), nil
	default:
		return notificationPreferenceDecision{}, fmt.Errorf("preferences are unsupported for channel type %q", channelType)
	}

	query := fmt.Sprintf(`
		SELECT %s, digest_frequency, quiet_hours_start, quiet_hours_end, quiet_hours_timezone
		FROM notification_preferences
		WHERE user_id = $1 AND organization_id = $2 AND event_type IN ($3, '*')
		ORDER BY CASE WHEN event_type = $3 THEN 0 ELSE 1 END
		LIMIT 1`, columnName) // #nosec G201 -- columnName comes from the closed switch above.

	preference := defaultNotificationPreference()
	err := database.QuerierFromContext(ctx, ne.pool).QueryRow(ctx, query, userID, orgID, eventType).Scan(
		&preference.Enabled, &preference.DigestFrequency, &preference.QuietHoursStart,
		&preference.QuietHoursEnd, &preference.QuietHoursTimezone,
	)
	if err == pgx.ErrNoRows {
		// No preference means default enabled.
		return preference, nil
	}
	if err != nil {
		return notificationPreferenceDecision{}, err
	}
	return preference, nil
}

func (ne *NotificationEngine) findUsersByRoleSlugs(ctx context.Context, orgID string, roles []string) ([]string, error) {
	query := `
		SELECT DISTINCT u.id
		FROM users u
		JOIN user_roles ur ON ur.user_id = u.id AND ur.organization_id = u.organization_id
		JOIN roles r ON r.id = ur.role_id
		WHERE u.organization_id = $1
		  AND r.slug = ANY($2::text[])
		  AND (r.organization_id IS NULL OR r.organization_id = $1)
		  AND r.deleted_at IS NULL
		  AND u.deleted_at IS NULL
		  AND u.status = 'active'`

	rows, err := database.QuerierFromContext(ctx, ne.pool).Query(ctx, query, orgID, roles)
	if err != nil {
		return nil, fmt.Errorf("query users by roles: %w", err)
	}
	defer rows.Close()

	var userIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		userIDs = append(userIDs, id)
	}
	return userIDs, rows.Err()
}

func (ne *NotificationEngine) findUsersByRoleIDs(ctx context.Context, orgID string, roleIDs []string) ([]string, error) {
	if len(roleIDs) == 0 {
		return nil, nil
	}
	for _, roleID := range roleIDs {
		if _, err := uuid.Parse(roleID); err != nil {
			return nil, fmt.Errorf("invalid recipient role ID")
		}
	}
	query := `
		SELECT DISTINCT u.id
		FROM users u
		JOIN user_roles ur ON ur.user_id = u.id AND ur.organization_id = u.organization_id
		JOIN roles r ON r.id = ur.role_id
		WHERE u.organization_id = $1
		  AND r.id = ANY($2::uuid[])
		  AND (r.organization_id IS NULL OR r.organization_id = $1)
		  AND r.deleted_at IS NULL
		  AND u.deleted_at IS NULL
		  AND u.status = 'active'
		ORDER BY u.id`
	rows, err := database.QuerierFromContext(ctx, ne.pool).Query(ctx, query, orgID, roleIDs)
	if err != nil {
		return nil, fmt.Errorf("query users by role IDs: %w", err)
	}
	defer rows.Close()
	var userIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan role recipient: %w", err)
		}
		userIDs = append(userIDs, id)
	}
	return userIDs, rows.Err()
}

func (ne *NotificationEngine) filterTenantUsers(ctx context.Context, orgID string, userIDs []string) ([]string, error) {
	if len(userIDs) == 0 {
		return nil, nil
	}
	for _, userID := range userIDs {
		if _, err := uuid.Parse(userID); err != nil {
			return nil, fmt.Errorf("invalid recipient user ID")
		}
	}
	rows, err := database.QuerierFromContext(ctx, ne.pool).Query(ctx, `
		SELECT id
		FROM users
		WHERE organization_id = $1
		  AND id = ANY($2::uuid[])
		  AND status = 'active'
		  AND deleted_at IS NULL
		ORDER BY id`, orgID, userIDs)
	if err != nil {
		return nil, fmt.Errorf("filter tenant recipients: %w", err)
	}
	defer rows.Close()
	var filtered []string
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			return nil, fmt.Errorf("scan tenant recipient: %w", err)
		}
		filtered = append(filtered, userID)
	}
	return filtered, rows.Err()
}

func (ne *NotificationEngine) createNotificationRecord(
	ctx context.Context,
	n Notification,
	ruleID, channelID string,
	event Event,
) (string, error) {
	eventPayload, err := json.Marshal(map[string]any{
		"event_type":  event.Type,
		"severity":    event.Severity,
		"entity_type": event.EntityType,
		"entity_id":   event.EntityID,
		"entity_ref":  event.EntityRef,
		"data":        event.Data,
		"timestamp":   event.Timestamp.UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		return "", fmt.Errorf("encode notification event: %w", err)
	}
	metadata, err := json.Marshal(map[string]string{"entity_id": event.EntityID, "entity_type": event.EntityType})
	if err != nil {
		return "", fmt.Errorf("encode notification metadata: %w", err)
	}
	query := `
		INSERT INTO notifications
			(organization_id, rule_id, event_type, event_payload, recipient_user_id,
			 channel_type, channel_id, subject, body, body_text, body_html, status,
			 metadata, event_id, delivery_key, digest_frequency, scheduled_for, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12,
		        $13, $14, $15, $16::digest_frequency, $17, $18)
		ON CONFLICT (organization_id, delivery_key) DO UPDATE
		SET delivery_key = EXCLUDED.delivery_key
		RETURNING id`

	deliveryKey := notificationDeliveryKey(event.ID, ruleID, n.RecipientUserID, channelID)
	var id string
	err = database.QuerierFromContext(ctx, ne.pool).QueryRow(ctx, query,
		n.OrgID, ruleID, n.EventType, eventPayload, n.RecipientUserID,
		n.ChannelType, channelID, n.Subject, n.Body, n.TextBody, n.HTMLBody, n.Status,
		metadata, event.ID, deliveryKey, n.DigestFrequency, n.ScheduledFor, n.CreatedAt,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("insert notification: %w", err)
	}
	return id, nil
}

func (ne *NotificationEngine) dispatchEmail(ctx context.Context, n Notification, _ map[string]any) error {
	if ne.email == nil {
		return notificationDeliveryError("transport_unavailable", true,
			fmt.Errorf("email delivery transport is not configured"))
	}
	// Look up the recipient's email address.
	var recipientEmail string
	err := database.QuerierFromContext(ctx, ne.pool).QueryRow(ctx, `
		SELECT email FROM users
		WHERE id = $1 AND organization_id = $2 AND status = 'active' AND deleted_at IS NULL`,
		n.RecipientUserID, n.OrgID).Scan(&recipientEmail)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return notificationDeliveryError("recipient_unavailable", true,
				fmt.Errorf("lookup recipient email: %w", err))
		}
		return fmt.Errorf("lookup recipient email: %w", err)
	}
	textBody, htmlBody := n.TextBody, n.HTMLBody
	if textBody == "" && htmlBody == "" {
		textBody = n.Body
	}
	return ne.email.Send(ctx, emailpkg.Message{
		To:       []string{recipientEmail},
		Subject:  n.Subject,
		TextBody: textBody,
		HTMLBody: htmlBody,
		Headers:  map[string]string{"X-ComplianceForge-Notification-ID": n.ID},
	})
}

func (ne *NotificationEngine) dispatchInApp(ctx context.Context, n Notification) error {
	// In-app notifications are already stored in the notifications table
	// with channel_type = 'in_app'. The record was created in createNotificationRecord.
	// Mark it as delivered immediately since it is visible on next poll/refresh.
	log.Info().
		Str("user_id", n.RecipientUserID).
		Str("notification_id", n.ID).
		Msg("in-app notification delivered")
	return nil
}

func (ne *NotificationEngine) dispatchWebhook(ctx context.Context, n Notification, config map[string]any) error {
	webhookURL, err := channelConfigString(config, "url", true)
	if err != nil {
		return notificationDeliveryError("configuration_invalid", true, err)
	}
	secret, err := channelConfigString(config, "secret", true)
	if err != nil {
		return notificationDeliveryError("configuration_invalid", true, err)
	}
	if len(secret) < 32 {
		return notificationDeliveryError("configuration_invalid", true,
			fmt.Errorf("webhook signing secret must contain at least 32 characters"))
	}
	destination, err := url.Parse(webhookURL)
	if err != nil {
		return notificationDeliveryError("configuration_invalid", true, fmt.Errorf("parse webhook URL: %w", err))
	}
	if err := safehttp.ValidateURL(destination, safehttp.Policy{}); err != nil {
		return notificationDeliveryError("configuration_invalid", true, fmt.Errorf("validate webhook URL: %w", err))
	}

	payload, err := json.Marshal(map[string]interface{}{
		"notification_id": n.ID,
		"event_type":      n.EventType,
		"subject":         n.Subject,
		"body":            n.Body,
		"org_id":          n.OrgID,
		"timestamp":       n.CreatedAt.Format(time.RFC3339),
	})
	if err != nil {
		return fmt.Errorf("marshal webhook payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "ComplianceForge-Webhook/1.0")
	timestamp := time.Now().UTC().Format(time.RFC3339Nano)
	req.Header.Set("X-ComplianceForge-Timestamp", timestamp)
	req.Header.Set("Idempotency-Key", n.ID)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp))
	_, _ = mac.Write([]byte("."))
	_, _ = mac.Write(payload)
	req.Header.Set("X-ComplianceForge-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))

	resp, err := ne.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("webhook POST failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		permanent := resp.StatusCode >= http.StatusBadRequest && resp.StatusCode < http.StatusInternalServerError &&
			resp.StatusCode != http.StatusRequestTimeout && resp.StatusCode != http.StatusConflict &&
			resp.StatusCode != http.StatusTooEarly && resp.StatusCode != http.StatusTooManyRequests
		return notificationDeliveryError("provider_rejected", permanent,
			fmt.Errorf("webhook returned status %d", resp.StatusCode))
	}

	log.Info().
		Int("status", resp.StatusCode).
		Str("notification_id", n.ID).
		Msg("webhook notification dispatched")

	return nil
}

func (ne *NotificationEngine) dispatchSlack(ctx context.Context, n Notification, config map[string]any) error {
	webhookURL, err := channelConfigString(config, "webhook_url", true)
	if err != nil {
		return notificationDeliveryError("configuration_invalid", true, err)
	}
	destination, err := url.Parse(webhookURL)
	if err != nil {
		return notificationDeliveryError("configuration_invalid", true, fmt.Errorf("parse Slack webhook URL: %w", err))
	}
	if err := safehttp.ValidateURL(destination, safehttp.Policy{AllowedHosts: []string{"hooks.slack.com"}}); err != nil {
		return notificationDeliveryError("configuration_invalid", true, fmt.Errorf("validate Slack webhook URL: %w", err))
	}
	if len(n.Subject) > 150 || len(n.Body) > 3000 {
		return notificationDeliveryError("content_invalid", true,
			fmt.Errorf("Slack notification content exceeds channel limits"))
	}

	slackPayload, err := json.Marshal(map[string]interface{}{
		"text": fmt.Sprintf("*%s*\n%s", n.Subject, n.Body),
		"blocks": []map[string]interface{}{
			{
				"type": "header",
				"text": map[string]string{
					"type": "plain_text",
					"text": n.Subject,
				},
			},
			{
				"type": "section",
				"text": map[string]string{
					"type": "mrkdwn",
					"text": n.Body,
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("marshal slack payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(slackPayload))
	if err != nil {
		return fmt.Errorf("create slack request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := ne.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("slack POST failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		permanent := resp.StatusCode >= http.StatusBadRequest && resp.StatusCode < http.StatusInternalServerError &&
			resp.StatusCode != http.StatusRequestTimeout && resp.StatusCode != http.StatusTooManyRequests
		return notificationDeliveryError("provider_rejected", permanent,
			fmt.Errorf("slack returned status %d", resp.StatusCode))
	}

	log.Info().
		Str("notification_id", n.ID).
		Msg("slack notification dispatched")

	return nil
}

func channelConfigString(config map[string]any, key string, required bool) (string, error) {
	value, exists := config[key]
	if !exists || value == nil {
		if required {
			return "", fmt.Errorf("notification channel %s is required", key)
		}
		return "", nil
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("notification channel %s must be a string", key)
	}
	text = strings.TrimSpace(text)
	if required && text == "" {
		return "", fmt.Errorf("notification channel %s is required", key)
	}
	return text, nil
}

func validateNotificationEvent(event Event) error {
	if event.ID != "" {
		if _, err := uuid.Parse(event.ID); err != nil {
			return fmt.Errorf("notification event ID is invalid")
		}
	}
	if _, err := uuid.Parse(event.OrgID); err != nil {
		return fmt.Errorf("notification event organization ID is invalid")
	}
	if event.EntityID != "" {
		if _, err := uuid.Parse(event.EntityID); err != nil {
			return fmt.Errorf("notification event entity ID is invalid")
		}
	}
	if len(event.Type) == 0 || len(event.Type) > 100 || !isSafeEventToken(event.Type) {
		return fmt.Errorf("notification event type is invalid")
	}
	if event.Severity != "" {
		switch event.Severity {
		case "low", "medium", "high", "critical":
		default:
			return fmt.Errorf("notification event severity is invalid")
		}
	}
	if len(event.EntityType) > 100 || (event.EntityType != "" && !isSafeEventToken(event.EntityType)) {
		return fmt.Errorf("notification event entity type is invalid")
	}
	if len(event.EntityRef) > 500 {
		return fmt.Errorf("notification event entity reference is too long")
	}
	encodedData, err := json.Marshal(event.Data)
	if err != nil {
		return fmt.Errorf("notification event data is invalid: %w", err)
	}
	if len(encodedData) > 256*1024 {
		return fmt.Errorf("notification event data exceeds maximum size")
	}
	return nil
}

func isSafeEventToken(value string) bool {
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') ||
			character == '.' || character == '_' || character == '-' {
			continue
		}
		return false
	}
	return true
}

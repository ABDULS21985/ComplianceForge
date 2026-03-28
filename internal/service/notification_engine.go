package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"text/template"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// NotificationRule represents a rule from the database.
type NotificationRule struct {
	ID              string                 `json:"id"`
	OrgID           string                 `json:"organization_id"`
	Name            string                 `json:"name"`
	EventType       string                 `json:"event_type"`
	SeverityFilter  []string               `json:"severity_filter"`
	Conditions      map[string]interface{} `json:"conditions"`
	ChannelIDs      []string               `json:"channel_ids"`
	RecipientType   string                 `json:"recipient_type"` // role, owner, user, dpo, ciso
	RecipientIDs    []string               `json:"recipient_ids"`
	TemplateID      string                 `json:"template_id"`
	IsActive        bool                   `json:"is_active"`
	CooldownMinutes int                    `json:"cooldown_minutes"`
}

// Notification represents a notification record.
type Notification struct {
	ID              string     `json:"id"`
	OrgID           string     `json:"organization_id"`
	EventType       string     `json:"event_type"`
	RecipientUserID string     `json:"recipient_user_id"`
	ChannelType     string     `json:"channel_type"`
	Subject         string     `json:"subject"`
	Body            string     `json:"body"`
	Status          string     `json:"status"`
	CreatedAt       time.Time  `json:"created_at"`
	ReadAt          *time.Time `json:"read_at"`
}

// NotificationEngine orchestrates the notification pipeline.
type NotificationEngine struct {
	pool   *pgxpool.Pool
	bus    *EventBus
	stopCh chan struct{}
}

// NewNotificationEngine creates a new NotificationEngine.
func NewNotificationEngine(pool *pgxpool.Pool, bus *EventBus) *NotificationEngine {
	return &NotificationEngine{
		pool:   pool,
		bus:    bus,
		stopCh: make(chan struct{}),
	}
}

// Start begins listening for events from the EventBus and processes them in a background goroutine.
func (ne *NotificationEngine) Start(ctx context.Context) {
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
}

// Stop signals the notification engine to shut down.
func (ne *NotificationEngine) Stop() {
	close(ne.stopCh)
}

// ProcessEvent is the core notification pipeline. It queries matching rules, evaluates
// conditions, determines recipients, renders templates, and dispatches notifications.
func (ne *NotificationEngine) ProcessEvent(ctx context.Context, event Event) error {
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
			continue
		}

		if len(recipientIDs) == 0 {
			log.Debug().Str("rule_id", rule.ID).Msg("no recipients found for rule")
			continue
		}

		// 5. Load and render the notification template.
		subject, body, err := ne.loadAndRenderTemplate(ctx, rule.TemplateID, event)
		if err != nil {
			log.Error().Err(err).Str("rule_id", rule.ID).Msg("failed to render template")
			continue
		}

		// Determine if this is a breach/regulatory event that bypasses preferences.
		bypassPreferences := ne.isBypassEvent(event.Type)

		// 6-7. For each recipient and channel, check preferences, create record, and dispatch.
		for _, recipientID := range recipientIDs {
			for _, channelID := range rule.ChannelIDs {
				channelType, channelConfig, err := ne.getChannelConfig(ctx, channelID)
				if err != nil {
					log.Error().Err(err).
						Str("channel_id", channelID).
						Msg("failed to load channel config")
					continue
				}

				// Check notification preferences unless bypass event.
				if !bypassPreferences {
					enabled, err := ne.checkUserPreference(ctx, recipientID, event.Type, channelType)
					if err != nil {
						log.Error().Err(err).
							Str("user_id", recipientID).
							Msg("failed to check notification preference")
					}
					if !enabled {
						log.Debug().
							Str("user_id", recipientID).
							Str("channel_type", channelType).
							Msg("user has disabled this notification channel, skipping")
						continue
					}
				}

				// Create notification record with status 'pending'.
				notification := Notification{
					OrgID:           event.OrgID,
					EventType:       event.Type,
					RecipientUserID: recipientID,
					ChannelType:     channelType,
					Subject:         subject,
					Body:            body,
					Status:          "pending",
					CreatedAt:       time.Now().UTC(),
				}

				notifID, err := ne.createNotificationRecord(ctx, notification, rule.ID, event.EntityID)
				if err != nil {
					log.Error().Err(err).Msg("failed to create notification record")
					continue
				}
				notification.ID = notifID

				// Dispatch via appropriate channel.
				if err := ne.Dispatch(ctx, notification, channelConfig); err != nil {
					log.Error().Err(err).
						Str("notification_id", notifID).
						Str("channel_type", channelType).
						Msg("failed to dispatch notification")
					ne.updateNotificationStatus(ctx, notifID, "failed")
					continue
				}

				ne.updateNotificationStatus(ctx, notifID, "sent")
			}
		}
	}

	return nil
}

// RenderTemplate parses a Go text/template string and executes it with the provided data.
func (ne *NotificationEngine) RenderTemplate(tmplStr string, data map[string]interface{}) (string, error) {
	tmpl, err := template.New("notification").Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}

	return buf.String(), nil
}

// DetermineRecipients resolves the list of user IDs that should receive the notification
// based on the rule's recipient_type setting.
func (ne *NotificationEngine) DetermineRecipients(ctx context.Context, rule NotificationRule, event Event) ([]string, error) {
	switch rule.RecipientType {
	case "user":
		// Directly specified user IDs.
		return rule.RecipientIDs, nil

	case "role":
		// Find all users in the org with any of the specified roles.
		return ne.findUsersByRoles(ctx, event.OrgID, rule.RecipientIDs)

	case "owner":
		// The entity owner is embedded in event data.
		if ownerID, ok := event.Data["owner_id"].(string); ok && ownerID != "" {
			return []string{ownerID}, nil
		}
		if assigneeID, ok := event.Data["assignee_id"].(string); ok && assigneeID != "" {
			return []string{assigneeID}, nil
		}
		return nil, nil

	case "dpo":
		return ne.findUsersByRoles(ctx, event.OrgID, []string{"dpo", "data_protection_officer"})

	case "ciso":
		return ne.findUsersByRoles(ctx, event.OrgID, []string{"ciso", "chief_information_security_officer"})

	default:
		return nil, fmt.Errorf("unknown recipient_type: %s", rule.RecipientType)
	}
}

// Dispatch sends a notification via the appropriate channel.
func (ne *NotificationEngine) Dispatch(ctx context.Context, notification Notification, channelConfig map[string]string) error {
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
		return fmt.Errorf("unsupported channel type: %s", notification.ChannelType)
	}
}

// --- Internal helper methods ---

func (ne *NotificationEngine) fetchMatchingRules(ctx context.Context, event Event) ([]NotificationRule, error) {
	query := `
		SELECT id, organization_id, name, event_type, severity_filter, conditions,
		       channel_ids, recipient_type, recipient_ids, template_id, is_active, cooldown_minutes
		FROM notification_rules
		WHERE is_active = true
		  AND organization_id = $1
		  AND (event_type = $2 OR event_type = '*')
		ORDER BY created_at ASC`

	rows, err := ne.pool.Query(ctx, query, event.OrgID, event.Type)
	if err != nil {
		return nil, fmt.Errorf("query notification rules: %w", err)
	}
	defer rows.Close()

	var rules []NotificationRule
	for rows.Next() {
		var rule NotificationRule
		var severityFilterJSON, conditionsJSON, channelIDsJSON, recipientIDsJSON []byte

		err := rows.Scan(
			&rule.ID, &rule.OrgID, &rule.Name, &rule.EventType,
			&severityFilterJSON, &conditionsJSON,
			&channelIDsJSON, &rule.RecipientType, &recipientIDsJSON,
			&rule.TemplateID, &rule.IsActive, &rule.CooldownMinutes,
		)
		if err != nil {
			return nil, fmt.Errorf("scan notification rule: %w", err)
		}

		if severityFilterJSON != nil {
			json.Unmarshal(severityFilterJSON, &rule.SeverityFilter)
		}
		if conditionsJSON != nil {
			json.Unmarshal(conditionsJSON, &rule.Conditions)
		}
		if channelIDsJSON != nil {
			json.Unmarshal(channelIDsJSON, &rule.ChannelIDs)
		}
		if recipientIDsJSON != nil {
			json.Unmarshal(recipientIDsJSON, &rule.RecipientIDs)
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
			  AND status = 'sent'
			  AND created_at > NOW() - ($2 || ' minutes')::interval
		)`

	var inCooldown bool
	err := ne.pool.QueryRow(ctx, query, ruleID, fmt.Sprintf("%d", cooldownMinutes)).Scan(&inCooldown)
	if err != nil {
		return false, err
	}
	return inCooldown, nil
}

func (ne *NotificationEngine) loadAndRenderTemplate(ctx context.Context, templateID string, event Event) (string, string, error) {
	var subjectTmpl, bodyTmpl string

	if templateID != "" {
		query := `SELECT subject_template, body_html_template FROM notification_templates WHERE id = $1`
		err := ne.pool.QueryRow(ctx, query, templateID).Scan(&subjectTmpl, &bodyTmpl)
		if err != nil && err != pgx.ErrNoRows {
			return "", "", fmt.Errorf("load template %s: %w", templateID, err)
		}
	}

	// Fallback to default templates if none found.
	if subjectTmpl == "" {
		subjectTmpl = "[ComplianceForge] {{.event_type}}: {{.entity_ref}}"
	}
	if bodyTmpl == "" {
		bodyTmpl = "Event: {{.event_type}}\nEntity: {{.entity_ref}} ({{.entity_type}})\nSeverity: {{.severity}}\nTime: {{.timestamp}}"
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
		return "", "", fmt.Errorf("render subject template: %w", err)
	}

	body, err := ne.RenderTemplate(bodyTmpl, data)
	if err != nil {
		return "", "", fmt.Errorf("render body template: %w", err)
	}

	return subject, body, nil
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

func (ne *NotificationEngine) getChannelConfig(ctx context.Context, channelID string) (string, map[string]string, error) {
	var channelType string
	var configJSON []byte

	query := `SELECT channel_type, configuration FROM notification_channels WHERE id = $1 AND is_active = true`
	err := ne.pool.QueryRow(ctx, query, channelID).Scan(&channelType, &configJSON)
	if err != nil {
		return "", nil, fmt.Errorf("load channel %s: %w", channelID, err)
	}

	config := make(map[string]string)
	if configJSON != nil {
		json.Unmarshal(configJSON, &config)
	}

	return channelType, config, nil
}

func (ne *NotificationEngine) checkUserPreference(ctx context.Context, userID, eventType, channelType string) (bool, error) {
	var columnName string
	switch channelType {
	case "email":
		columnName = "email_enabled"
	case "in_app":
		columnName = "in_app_enabled"
	case "slack":
		columnName = "slack_enabled"
	default:
		// Unknown channel types are enabled by default.
		return true, nil
	}

	query := fmt.Sprintf(`
		SELECT %s FROM notification_preferences
		WHERE user_id = $1
		LIMIT 1`, columnName)

	var enabled bool
	err := ne.pool.QueryRow(ctx, query, userID).Scan(&enabled)
	if err == pgx.ErrNoRows {
		// No preference means default enabled.
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return enabled, nil
}

func (ne *NotificationEngine) findUsersByRoles(ctx context.Context, orgID string, roles []string) ([]string, error) {
	query := `
		SELECT DISTINCT u.id FROM users u
		WHERE u.organization_id = $1
		  AND u.role = ANY($2)
		  AND u.deleted_at IS NULL`

	rows, err := ne.pool.Query(ctx, query, orgID, roles)
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

func (ne *NotificationEngine) createNotificationRecord(ctx context.Context, n Notification, ruleID, entityID string) (string, error) {
	query := `
		INSERT INTO notifications
			(organization_id, event_type, recipient_user_id, channel_type, subject, body, status, rule_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id`

	var id string
	err := ne.pool.QueryRow(ctx, query,
		n.OrgID, n.EventType, n.RecipientUserID, n.ChannelType,
		n.Subject, n.Body, n.Status, ruleID, n.CreatedAt,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("insert notification: %w", err)
	}
	return id, nil
}

func (ne *NotificationEngine) updateNotificationStatus(ctx context.Context, notifID, status string) {
	query := `UPDATE notifications SET status = $1, updated_at = NOW() WHERE id = $2`
	if _, err := ne.pool.Exec(ctx, query, status, notifID); err != nil {
		log.Error().Err(err).Str("notification_id", notifID).Msg("failed to update notification status")
	}
}

func (ne *NotificationEngine) dispatchEmail(ctx context.Context, n Notification, config map[string]string) error {
	// Look up the recipient's email address.
	var email string
	err := ne.pool.QueryRow(ctx, `SELECT email FROM users WHERE id = $1`, n.RecipientUserID).Scan(&email)
	if err != nil {
		return fmt.Errorf("lookup recipient email: %w", err)
	}

	log.Info().
		Str("to", email).
		Str("subject", n.Subject).
		Str("notification_id", n.ID).
		Msg("dispatching email notification")

	// In production, use net/smtp or an email service provider.
	// This is a placeholder that logs the dispatch.
	return nil
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

func (ne *NotificationEngine) dispatchWebhook(ctx context.Context, n Notification, config map[string]string) error {
	webhookURL := config["url"]
	secret := config["secret"]

	if webhookURL == "" {
		return fmt.Errorf("webhook URL not configured")
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

	// Sign payload with HMAC-SHA256 if a secret is configured.
	if secret != "" {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(payload)
		signature := hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-ComplianceForge-Signature", "sha256="+signature)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook POST failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	log.Info().
		Str("url", webhookURL).
		Int("status", resp.StatusCode).
		Str("notification_id", n.ID).
		Msg("webhook notification dispatched")

	return nil
}

func (ne *NotificationEngine) dispatchSlack(ctx context.Context, n Notification, config map[string]string) error {
	webhookURL := config["webhook_url"]
	if webhookURL == "" {
		return fmt.Errorf("slack webhook URL not configured")
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

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("slack POST failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack returned status %d", resp.StatusCode)
	}

	log.Info().
		Str("notification_id", n.ID).
		Msg("slack notification dispatched")

	return nil
}

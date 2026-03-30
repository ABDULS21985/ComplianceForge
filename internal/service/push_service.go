package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

var (
	ErrTokenNotFound      = fmt.Errorf("push token not found")
	ErrTooManyDevices     = fmt.Errorf("maximum of 5 devices per user")
	ErrPushSendFailed     = fmt.Errorf("failed to deliver push notification")
	ErrQuietHoursActive   = fmt.Errorf("notification suppressed during quiet hours")
)

// ---------------------------------------------------------------------------
// Domain types
// ---------------------------------------------------------------------------

// PushToken represents a registered mobile device token.
type PushToken struct {
	ID         string  `json:"id"`
	UserID     string  `json:"user_id"`
	Platform   string  `json:"platform"` // ios, android, web
	TokenHash  string  `json:"-"`
	DeviceInfo string  `json:"device_info"`
	IsActive   bool    `json:"is_active"`
	LastUsedAt string  `json:"last_used_at"`
	CreatedAt  string  `json:"created_at"`
}

// PushNotification is the payload for a push notification.
type PushNotification struct {
	Title    string                 `json:"title"`
	Body     string                 `json:"body"`
	Category string                 `json:"category"` // alert, reminder, approval, incident
	Priority string                 `json:"priority"`  // critical, high, normal, low
	Data     map[string]interface{} `json:"data"`
	Badge    int                    `json:"badge"`
	Sound    string                 `json:"sound"`
}

// MobilePreferences holds a user's push notification preferences.
type MobilePreferences struct {
	UserID           string `json:"user_id"`
	PushEnabled      bool   `json:"push_enabled"`
	AlertsEnabled    bool   `json:"alerts_enabled"`
	RemindersEnabled bool   `json:"reminders_enabled"`
	ApprovalsEnabled bool   `json:"approvals_enabled"`
	IncidentsEnabled bool   `json:"incidents_enabled"`
	QuietHoursStart  string `json:"quiet_hours_start"` // HH:MM
	QuietHoursEnd    string `json:"quiet_hours_end"`
	QuietHoursZone   string `json:"quiet_hours_zone"`  // e.g. Europe/Berlin
	UpdatedAt        string `json:"updated_at"`
}

// MobileDashboard is a condensed dashboard for mobile clients.
type MobileDashboard struct {
	OpenRisks        int     `json:"open_risks"`
	CriticalRisks    int     `json:"critical_risks"`
	OverdueItems     int     `json:"overdue_items"`
	PendingApprovals int     `json:"pending_approvals"`
	ActiveIncidents  int     `json:"active_incidents"`
	ComplianceScore  float64 `json:"compliance_score"`
	UpcomingDeadlines int    `json:"upcoming_deadlines"`
}

// MobileApproval is a condensed approval item for mobile.
type MobileApproval struct {
	ID          string `json:"id"`
	EntityType  string `json:"entity_type"`
	EntityRef   string `json:"entity_ref"`
	Title       string `json:"title"`
	RequestedBy string `json:"requested_by"`
	RequestedAt string `json:"requested_at"`
	Priority    string `json:"priority"`
}

// MobileIncident is a condensed incident item for mobile.
type MobileIncident struct {
	ID          string `json:"id"`
	IncidentRef string `json:"incident_ref"`
	Title       string `json:"title"`
	Severity    string `json:"severity"`
	Status      string `json:"status"`
	ReportedAt  string `json:"reported_at"`
}

// MobileDeadline is a condensed deadline for mobile.
type MobileDeadline struct {
	EntityType string `json:"entity_type"`
	EntityRef  string `json:"entity_ref"`
	Title      string `json:"title"`
	DueDate    string `json:"due_date"`
	Priority   string `json:"priority"`
	DaysLeft   int    `json:"days_left"`
}

// DeliveryLog records each push delivery attempt.
type DeliveryLog struct {
	ID        string `json:"id"`
	TokenID   string `json:"token_id"`
	UserID    string `json:"user_id"`
	Title     string `json:"title"`
	Status    string `json:"status"` // delivered, failed, suppressed
	Error     string `json:"error"`
	CreatedAt string `json:"created_at"`
}

// ---------------------------------------------------------------------------
// Service
// ---------------------------------------------------------------------------

// PushService manages mobile push notifications and mobile API endpoints.
type PushService struct {
	pool       *pgxpool.Pool
	httpClient *http.Client
}

// NewPushService creates a PushService.
func NewPushService(pool *pgxpool.Pool) *PushService {
	return &PushService{
		pool:       pool,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// ---------------------------------------------------------------------------
// Token management
// ---------------------------------------------------------------------------

// RegisterToken registers a device token for push notifications.
func (s *PushService) RegisterToken(ctx context.Context, userID, platform, token, deviceInfo string) (*PushToken, error) {
	tokenHash := hashPushToken(token)

	// Count existing active tokens
	var count int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM push_tokens WHERE user_id = $1 AND is_active = true`, userID).Scan(&count)
	if err != nil {
		return nil, fmt.Errorf("push: count tokens: %w", err)
	}
	if count >= 5 {
		// Deactivate oldest
		_, _ = s.pool.Exec(ctx, `
			UPDATE push_tokens SET is_active = false
			WHERE id = (
				SELECT id FROM push_tokens WHERE user_id = $1 AND is_active = true
				ORDER BY last_used_at ASC LIMIT 1
			)`, userID)
	}

	var pt PushToken
	err = s.pool.QueryRow(ctx, `
		INSERT INTO push_tokens (id, user_id, platform, token_hash, device_info, is_active, last_used_at, created_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, true, NOW(), NOW())
		ON CONFLICT (user_id, token_hash) DO UPDATE
		  SET is_active = true, device_info = EXCLUDED.device_info, platform = EXCLUDED.platform, last_used_at = NOW()
		RETURNING id, user_id, platform, token_hash, device_info, is_active, last_used_at, created_at`,
		userID, platform, tokenHash, deviceInfo).Scan(
		&pt.ID, &pt.UserID, &pt.Platform, &pt.TokenHash, &pt.DeviceInfo,
		&pt.IsActive, &pt.LastUsedAt, &pt.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("push: register token: %w", err)
	}
	log.Info().Str("user_id", userID).Str("platform", platform).Msg("push: token registered")
	return &pt, nil
}

// UnregisterToken deactivates a device token.
func (s *PushService) UnregisterToken(ctx context.Context, userID, tokenHash string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE push_tokens SET is_active = false WHERE user_id = $1 AND token_hash = $2`,
		userID, tokenHash)
	if err != nil {
		return fmt.Errorf("push: unregister token: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrTokenNotFound
	}
	log.Info().Str("user_id", userID).Msg("push: token unregistered")
	return nil
}

func hashPushToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// ---------------------------------------------------------------------------
// Send push
// ---------------------------------------------------------------------------

// SendPush delivers a push notification to all active tokens for a user.
func (s *PushService) SendPush(ctx context.Context, userID string, notification PushNotification) error {
	// Check preferences
	prefs, err := s.GetMobilePreferences(ctx, userID)
	if err == nil {
		if !prefs.PushEnabled {
			log.Debug().Str("user_id", userID).Msg("push: notifications disabled for user")
			return nil
		}
		if !s.categoryEnabled(prefs, notification.Category) {
			log.Debug().Str("user_id", userID).Str("category", notification.Category).Msg("push: category disabled")
			return nil
		}
		if notification.Priority != "critical" && s.isQuietHours(prefs) {
			log.Debug().Str("user_id", userID).Msg("push: suppressed during quiet hours")
			s.logDelivery(ctx, "", userID, notification.Title, "suppressed", "quiet hours")
			return nil
		}
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, platform, token_hash FROM push_tokens
		WHERE user_id = $1 AND is_active = true`, userID)
	if err != nil {
		return fmt.Errorf("push: fetch tokens: %w", err)
	}
	defer rows.Close()

	var sentCount int
	for rows.Next() {
		var tokenID, platform, tHash string
		if err := rows.Scan(&tokenID, &platform, &tHash); err != nil {
			continue
		}

		err := s.deliverPush(ctx, platform, tHash, notification)
		if err != nil {
			log.Warn().Err(err).Str("token_id", tokenID).Msg("push: delivery failed")
			s.logDelivery(ctx, tokenID, userID, notification.Title, "failed", err.Error())
			// Deactivate invalid tokens
			if isInvalidTokenError(err) {
				_, _ = s.pool.Exec(ctx, "UPDATE push_tokens SET is_active = false WHERE id = $1", tokenID)
			}
			continue
		}
		s.logDelivery(ctx, tokenID, userID, notification.Title, "delivered", "")
		_, _ = s.pool.Exec(ctx, "UPDATE push_tokens SET last_used_at = NOW() WHERE id = $1", tokenID)
		sentCount++
	}

	log.Info().Str("user_id", userID).Int("sent", sentCount).Msg("push: notification sent")
	return nil
}

// SendBulkPush sends a notification to multiple users.
func (s *PushService) SendBulkPush(ctx context.Context, userIDs []string, notification PushNotification) error {
	for _, uid := range userIDs {
		if err := s.SendPush(ctx, uid, notification); err != nil {
			log.Warn().Err(err).Str("user_id", uid).Msg("push: bulk send failed for user")
		}
	}
	return nil
}

// deliverPush sends a push via FCM/APNs (placeholder HTTP POST).
func (s *PushService) deliverPush(_ context.Context, platform, tokenHash string, n PushNotification) error {
	payload := map[string]interface{}{
		"to":       tokenHash,
		"title":    n.Title,
		"body":     n.Body,
		"category": n.Category,
		"priority": n.Priority,
		"data":     n.Data,
		"badge":    n.Badge,
		"sound":    n.Sound,
	}
	body, _ := json.Marshal(payload)

	var endpoint string
	switch platform {
	case "ios":
		endpoint = "https://api.push.apple.com/3/device/" + tokenHash
	case "android":
		endpoint = "https://fcm.googleapis.com/fcm/send"
	case "web":
		endpoint = "https://fcm.googleapis.com/fcm/send"
	default:
		return fmt.Errorf("unsupported platform: %s", platform)
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer <configured-push-key>")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("push: HTTP error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("push: HTTP %d from %s", resp.StatusCode, platform)
	}
	return nil
}

func isInvalidTokenError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "410") || strings.Contains(msg, "404") || strings.Contains(msg, "InvalidRegistration")
}

func (s *PushService) logDelivery(ctx context.Context, tokenID, userID, title, status, errMsg string) {
	_, _ = s.pool.Exec(ctx, `
		INSERT INTO push_delivery_log (id, token_id, user_id, title, status, error, created_at)
		VALUES (gen_random_uuid(), NULLIF($1,''), $2, $3, $4, $5, NOW())`,
		tokenID, userID, title, status, errMsg)
}

func (s *PushService) categoryEnabled(prefs *MobilePreferences, category string) bool {
	switch category {
	case "alert":
		return prefs.AlertsEnabled
	case "reminder":
		return prefs.RemindersEnabled
	case "approval":
		return prefs.ApprovalsEnabled
	case "incident":
		return prefs.IncidentsEnabled
	default:
		return true
	}
}

func (s *PushService) isQuietHours(prefs *MobilePreferences) bool {
	if prefs.QuietHoursStart == "" || prefs.QuietHoursEnd == "" {
		return false
	}
	loc, err := time.LoadLocation(prefs.QuietHoursZone)
	if err != nil {
		loc = time.UTC
	}
	now := time.Now().In(loc)
	nowMin := now.Hour()*60 + now.Minute()

	startH, startM := parseHHMM(prefs.QuietHoursStart)
	endH, endM := parseHHMM(prefs.QuietHoursEnd)
	startMin := startH*60 + startM
	endMin := endH*60 + endM

	if startMin <= endMin {
		return nowMin >= startMin && nowMin < endMin
	}
	// Wraps midnight
	return nowMin >= startMin || nowMin < endMin
}

func parseHHMM(s string) (int, int) {
	var h, m int
	fmt.Sscanf(s, "%d:%d", &h, &m)
	return h, m
}

// ---------------------------------------------------------------------------
// Preferences
// ---------------------------------------------------------------------------

// GetMobilePreferences retrieves push preferences for a user.
func (s *PushService) GetMobilePreferences(ctx context.Context, userID string) (*MobilePreferences, error) {
	var p MobilePreferences
	err := s.pool.QueryRow(ctx, `
		SELECT user_id, push_enabled, alerts_enabled, reminders_enabled,
			   approvals_enabled, incidents_enabled,
			   quiet_hours_start, quiet_hours_end, quiet_hours_zone, updated_at
		FROM mobile_preferences
		WHERE user_id = $1`, userID).Scan(
		&p.UserID, &p.PushEnabled, &p.AlertsEnabled, &p.RemindersEnabled,
		&p.ApprovalsEnabled, &p.IncidentsEnabled,
		&p.QuietHoursStart, &p.QuietHoursEnd, &p.QuietHoursZone, &p.UpdatedAt)
	if err == pgx.ErrNoRows {
		// Return defaults
		return &MobilePreferences{
			UserID: userID, PushEnabled: true, AlertsEnabled: true,
			RemindersEnabled: true, ApprovalsEnabled: true, IncidentsEnabled: true,
			QuietHoursStart: "22:00", QuietHoursEnd: "07:00", QuietHoursZone: "UTC",
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("push: get preferences: %w", err)
	}
	return &p, nil
}

// UpdateMobilePreferences updates push preferences.
func (s *PushService) UpdateMobilePreferences(ctx context.Context, userID string, prefs MobilePreferences) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO mobile_preferences
			(user_id, push_enabled, alerts_enabled, reminders_enabled,
			 approvals_enabled, incidents_enabled,
			 quiet_hours_start, quiet_hours_end, quiet_hours_zone, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
		ON CONFLICT (user_id) DO UPDATE
		  SET push_enabled = $2, alerts_enabled = $3, reminders_enabled = $4,
			  approvals_enabled = $5, incidents_enabled = $6,
			  quiet_hours_start = $7, quiet_hours_end = $8, quiet_hours_zone = $9,
			  updated_at = NOW()`,
		userID, prefs.PushEnabled, prefs.AlertsEnabled, prefs.RemindersEnabled,
		prefs.ApprovalsEnabled, prefs.IncidentsEnabled,
		prefs.QuietHoursStart, prefs.QuietHoursEnd, prefs.QuietHoursZone)
	if err != nil {
		return fmt.Errorf("push: update preferences: %w", err)
	}
	log.Info().Str("user_id", userID).Msg("push: preferences updated")
	return nil
}

// ---------------------------------------------------------------------------
// Mobile dashboard endpoints
// ---------------------------------------------------------------------------

// GetMobileDashboard returns condensed metrics for the mobile dashboard.
func (s *PushService) GetMobileDashboard(ctx context.Context, orgID, userID string) (*MobileDashboard, error) {
	var d MobileDashboard
	err := s.pool.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*) FROM risks WHERE organization_id = $1 AND status = 'open'),
			(SELECT COUNT(*) FROM risks WHERE organization_id = $1 AND status = 'open' AND inherent_risk_level = 'critical'),
			(SELECT COUNT(*) FROM calendar_events WHERE organization_id = $1 AND status = 'overdue'),
			(SELECT COUNT(*) FROM workflow_instances WHERE organization_id = $1 AND status = 'pending_approval'
			   AND current_approver = $2),
			(SELECT COUNT(*) FROM incidents WHERE organization_id = $1 AND status IN ('open','investigating')),
			COALESCE((SELECT AVG(compliance_score) FROM compliance_scores WHERE organization_id = $1), 0),
			(SELECT COUNT(*) FROM calendar_events WHERE organization_id = $1 AND status = 'pending'
			   AND due_date BETWEEN CURRENT_DATE AND CURRENT_DATE + INTERVAL '7 days')
		`, orgID, userID).Scan(
		&d.OpenRisks, &d.CriticalRisks, &d.OverdueItems, &d.PendingApprovals,
		&d.ActiveIncidents, &d.ComplianceScore, &d.UpcomingDeadlines)
	if err != nil {
		return nil, fmt.Errorf("push: mobile dashboard: %w", err)
	}
	return &d, nil
}

// GetMobileApprovals returns pending approvals condensed for mobile.
func (s *PushService) GetMobileApprovals(ctx context.Context, orgID, userID string) ([]MobileApproval, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT wi.id, wi.entity_type, wi.entity_ref, wi.title, wi.initiated_by, wi.created_at, wi.priority
		FROM workflow_instances wi
		WHERE wi.organization_id = $1 AND wi.status = 'pending_approval' AND wi.current_approver = $2
		ORDER BY wi.created_at DESC
		LIMIT 20`, orgID, userID)
	if err != nil {
		return nil, fmt.Errorf("push: mobile approvals: %w", err)
	}
	defer rows.Close()

	var approvals []MobileApproval
	for rows.Next() {
		var a MobileApproval
		if err := rows.Scan(&a.ID, &a.EntityType, &a.EntityRef, &a.Title,
			&a.RequestedBy, &a.RequestedAt, &a.Priority); err != nil {
			continue
		}
		approvals = append(approvals, a)
	}
	return approvals, nil
}

// GetMobileIncidents returns active incidents condensed for mobile.
func (s *PushService) GetMobileIncidents(ctx context.Context, orgID string) ([]MobileIncident, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, incident_ref, title, severity, status, reported_at
		FROM incidents
		WHERE organization_id = $1 AND status IN ('open','investigating')
		ORDER BY severity_ord(severity) ASC, reported_at DESC
		LIMIT 20`, orgID)
	if err != nil {
		return nil, fmt.Errorf("push: mobile incidents: %w", err)
	}
	defer rows.Close()

	var incidents []MobileIncident
	for rows.Next() {
		var i MobileIncident
		if err := rows.Scan(&i.ID, &i.IncidentRef, &i.Title, &i.Severity, &i.Status, &i.ReportedAt); err != nil {
			continue
		}
		incidents = append(incidents, i)
	}
	return incidents, nil
}

// GetMobileDeadlines returns upcoming deadlines condensed for mobile.
func (s *PushService) GetMobileDeadlines(ctx context.Context, orgID, userID string, days int) ([]MobileDeadline, error) {
	if days < 1 {
		days = 7
	}
	rows, err := s.pool.Query(ctx, `
		SELECT entity_type, source_ref, title, due_date, priority,
			   EXTRACT(DAY FROM due_date::timestamp - CURRENT_TIMESTAMP)::int AS days_left
		FROM calendar_events
		WHERE organization_id = $1
		  AND status = 'pending'
		  AND due_date BETWEEN CURRENT_DATE AND CURRENT_DATE + ($2 || ' days')::interval
		  AND (assigned_to = $3 OR assigned_to IS NULL)
		ORDER BY due_date ASC
		LIMIT 20`, orgID, days, userID)
	if err != nil {
		return nil, fmt.Errorf("push: mobile deadlines: %w", err)
	}
	defer rows.Close()

	var deadlines []MobileDeadline
	for rows.Next() {
		var d MobileDeadline
		if err := rows.Scan(&d.EntityType, &d.EntityRef, &d.Title, &d.DueDate,
			&d.Priority, &d.DaysLeft); err != nil {
			continue
		}
		deadlines = append(deadlines, d)
	}
	return deadlines, nil
}

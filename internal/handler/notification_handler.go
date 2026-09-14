package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"

	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/middleware"
	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/pkg/safehttp"
	"github.com/complianceforge/platform/internal/pkg/secretbox"
	"github.com/complianceforge/platform/internal/service"
)

// NotificationHandler handles notification-related API endpoints.
type NotificationHandler struct {
	pool      *pgxpool.Pool
	engine    *service.NotificationEngine
	protector *secretbox.Box
}

type notificationPreference struct {
	ID                 string    `json:"id"`
	UserID             string    `json:"user_id"`
	OrganizationID     string    `json:"organization_id"`
	EventType          string    `json:"event_type"`
	EmailEnabled       bool      `json:"email_enabled"`
	InAppEnabled       bool      `json:"in_app_enabled"`
	SlackEnabled       bool      `json:"slack_enabled"`
	DigestFrequency    string    `json:"digest_frequency"`
	QuietHoursStart    *string   `json:"quiet_hours_start"`
	QuietHoursEnd      *string   `json:"quiet_hours_end"`
	QuietHoursTimezone *string   `json:"quiet_hours_timezone"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type notificationRuleInput struct {
	Name            string                 `json:"name"`
	EventType       string                 `json:"event_type"`
	SeverityFilter  []string               `json:"severity_filter"`
	Conditions      map[string]interface{} `json:"conditions"`
	ChannelIDs      []string               `json:"channel_ids"`
	RecipientType   string                 `json:"recipient_type"`
	RecipientIDs    []string               `json:"recipient_ids"`
	TemplateID      *string                `json:"template_id"`
	IsActive        *bool                  `json:"is_active"`
	CooldownMinutes int                    `json:"cooldown_minutes"`
}

type notificationRuleResponse struct {
	ID              string                 `json:"id"`
	OrgID           string                 `json:"organization_id"`
	Name            string                 `json:"name"`
	EventType       string                 `json:"event_type"`
	SeverityFilter  []string               `json:"severity_filter"`
	Conditions      map[string]interface{} `json:"conditions"`
	ChannelIDs      []string               `json:"channel_ids"`
	RecipientType   string                 `json:"recipient_type"`
	RecipientIDs    []string               `json:"recipient_ids"`
	TemplateID      *string                `json:"template_id"`
	IsActive        bool                   `json:"is_active"`
	CooldownMinutes int                    `json:"cooldown_minutes"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
}

type notificationChannelInput struct {
	Name        string         `json:"name"`
	ChannelType string         `json:"channel_type"`
	Config      map[string]any `json:"config"`
	IsActive    *bool          `json:"is_active"`
}

type notificationChannelResponse struct {
	ID          string         `json:"id"`
	OrgID       string         `json:"organization_id"`
	Name        string         `json:"name"`
	ChannelType string         `json:"channel_type"`
	Config      map[string]any `json:"config"`
	IsActive    bool           `json:"is_active"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

type notificationTemplateInput struct {
	Name             string   `json:"name"`
	EventType        string   `json:"event_type"`
	SubjectTemplate  string   `json:"subject_template"`
	BodyHTMLTemplate string   `json:"body_html_template"`
	BodyTextTemplate string   `json:"body_text_template"`
	Variables        []string `json:"variables"`
}

type notificationTemplateResponse struct {
	ID               string    `json:"id"`
	OrgID            *string   `json:"organization_id"`
	Name             string    `json:"name"`
	EventType        string    `json:"event_type"`
	SubjectTemplate  string    `json:"subject_template"`
	BodyHTMLTemplate string    `json:"body_html_template"`
	BodyTextTemplate string    `json:"body_text_template"`
	Variables        []string  `json:"variables"`
	IsSystem         bool      `json:"is_system"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// NewNotificationHandler creates a new NotificationHandler.
func NewNotificationHandler(
	pool *pgxpool.Pool,
	engine *service.NotificationEngine,
	protectors ...*secretbox.Box,
) *NotificationHandler {
	var protector *secretbox.Box
	if len(protectors) > 0 {
		protector = protectors[0]
	}
	return &NotificationHandler{pool: pool, engine: engine, protector: protector}
}

// Ready reports whether the handler has all dependencies needed for both
// persisted notification APIs and channel test delivery.
func (h *NotificationHandler) Ready() bool {
	return h != nil && h.pool != nil && h.engine != nil && h.engine.Ready() && h.protector != nil
}

// --------------------------------------------------------------------------
// User-facing endpoints
// --------------------------------------------------------------------------

// ListNotifications handles GET /notifications.
// Returns the authenticated user's in-app notifications, paginated, newest first.
// The X-Unread-Count response header contains the total unread count for badge display.
func (h *NotificationHandler) ListNotifications(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserIDFromContext(r.Context())
	orgID := middleware.GetOrgIDFromContext(r.Context())
	if userID == "" || orgID == "" {
		writeError(w, http.StatusUnauthorized, "Authentication required", "")
		return
	}

	pagination := parsePagination(r)
	offset := (pagination.Page - 1) * pagination.PageSize

	// Count total notifications for this user.
	var total int
	err := database.QuerierFromContext(r.Context(), h.pool).QueryRow(r.Context(),
		`SELECT COUNT(*) FROM notifications
		 WHERE recipient_user_id = $1 AND organization_id = $2 AND channel_type = 'in_app'`,
		userID, orgID,
	).Scan(&total)
	if err != nil {
		writeNotificationInternalError(w, r, "count notifications", "Failed to count notifications", err)
		return
	}

	// Fetch paginated notifications, newest first.
	rows, err := database.QuerierFromContext(r.Context(), h.pool).Query(r.Context(),
		`SELECT id, organization_id, event_type, recipient_user_id, channel_type,
		        subject, body, status, created_at, read_at
		 FROM notifications
		 WHERE recipient_user_id = $1 AND organization_id = $2 AND channel_type = 'in_app'
		 ORDER BY created_at DESC
		 LIMIT $3 OFFSET $4`,
		userID, orgID, pagination.PageSize, offset,
	)
	if err != nil {
		writeNotificationInternalError(w, r, "list notifications", "Failed to list notifications", err)
		return
	}
	defer rows.Close()

	notifications := make([]service.Notification, 0)
	for rows.Next() {
		var n service.Notification
		if err := rows.Scan(
			&n.ID, &n.OrgID, &n.EventType, &n.RecipientUserID, &n.ChannelType,
			&n.Subject, &n.Body, &n.Status, &n.CreatedAt, &n.ReadAt,
		); err != nil {
			writeNotificationInternalError(w, r, "scan notification", "Failed to list notifications", err)
			return
		}
		notifications = append(notifications, n)
	}
	if err := rows.Err(); err != nil {
		writeNotificationInternalError(w, r, "iterate notifications", "Failed to list notifications", err)
		return
	}

	// Fetch unread count and include in header.
	var unreadCount int
	err = database.QuerierFromContext(r.Context(), h.pool).QueryRow(r.Context(),
		`SELECT COUNT(*) FROM notifications
		 WHERE recipient_user_id = $1 AND organization_id = $2
		   AND channel_type = 'in_app' AND read_at IS NULL`,
		userID, orgID,
	).Scan(&unreadCount)
	if err != nil {
		log.Error().Err(err).Msg("failed to count unread notifications")
	}

	w.Header().Set("X-Unread-Count", strconv.Itoa(unreadCount))

	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": notifications,
		"pagination": models.PaginationResponse{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}

// MarkAsRead handles PUT /notifications/{id}/read.
func (h *NotificationHandler) MarkAsRead(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserIDFromContext(r.Context())
	orgID := middleware.GetOrgIDFromContext(r.Context())
	if userID == "" || orgID == "" {
		writeError(w, http.StatusUnauthorized, "Authentication required", "")
		return
	}

	notifID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(notifID); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid notification ID", "")
		return
	}

	result, err := database.QuerierFromContext(r.Context(), h.pool).Exec(r.Context(),
		`UPDATE notifications SET read_at = COALESCE(read_at, NOW())
		 WHERE id = $1 AND recipient_user_id = $2 AND organization_id = $3
		   AND channel_type = 'in_app'`,
		notifID, userID, orgID,
	)
	if err != nil {
		writeNotificationInternalError(w, r, "mark notification read", "Failed to mark notification as read", err)
		return
	}

	if result.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "Notification not found", "")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Notification marked as read"})
}

// MarkAllAsRead handles PUT /notifications/read-all.
func (h *NotificationHandler) MarkAllAsRead(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserIDFromContext(r.Context())
	orgID := middleware.GetOrgIDFromContext(r.Context())
	if userID == "" || orgID == "" {
		writeError(w, http.StatusUnauthorized, "Authentication required", "")
		return
	}

	result, err := database.QuerierFromContext(r.Context(), h.pool).Exec(r.Context(),
		`UPDATE notifications SET read_at = NOW()
		 WHERE recipient_user_id = $1 AND organization_id = $2
		   AND channel_type = 'in_app' AND read_at IS NULL`,
		userID, orgID,
	)
	if err != nil {
		writeNotificationInternalError(w, r, "mark all notifications read", "Failed to mark all notifications as read", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "All notifications marked as read",
		"count":   result.RowsAffected(),
	})
}

// GetUnreadCount handles GET /notifications/unread-count.
// Returns {"count": N} for the notification bell badge.
func (h *NotificationHandler) GetUnreadCount(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserIDFromContext(r.Context())
	orgID := middleware.GetOrgIDFromContext(r.Context())
	if userID == "" || orgID == "" {
		writeError(w, http.StatusUnauthorized, "Authentication required", "")
		return
	}

	var count int
	err := database.QuerierFromContext(r.Context(), h.pool).QueryRow(r.Context(),
		`SELECT COUNT(*) FROM notifications
		 WHERE recipient_user_id = $1 AND organization_id = $2
		   AND channel_type = 'in_app' AND read_at IS NULL`,
		userID, orgID,
	).Scan(&count)
	if err != nil {
		writeNotificationInternalError(w, r, "count unread notifications", "Failed to count unread notifications", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]int{"count": count})
}

// GetPreferences handles GET /notifications/preferences.
func (h *NotificationHandler) GetPreferences(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserIDFromContext(r.Context())
	orgID := middleware.GetOrgIDFromContext(r.Context())
	if userID == "" || orgID == "" {
		writeError(w, http.StatusUnauthorized, "Authentication required", "")
		return
	}

	var p notificationPreference
	err := database.QuerierFromContext(r.Context(), h.pool).QueryRow(r.Context(),
		`SELECT id, user_id, organization_id, event_type,
		        email_enabled, in_app_enabled, slack_enabled,
		        digest_frequency, quiet_hours_start, quiet_hours_end, quiet_hours_timezone,
		        created_at, updated_at
		 FROM notification_preferences
		 WHERE user_id = $1 AND organization_id = $2 AND event_type = '*'`,
		userID, orgID,
	).Scan(&p.ID, &p.UserID, &p.OrganizationID, &p.EventType,
		&p.EmailEnabled, &p.InAppEnabled, &p.SlackEnabled,
		&p.DigestFrequency, &p.QuietHoursStart, &p.QuietHoursEnd, &p.QuietHoursTimezone,
		&p.CreatedAt, &p.UpdatedAt)
	if err == pgx.ErrNoRows {
		writeJSON(w, http.StatusOK, map[string]interface{}{"data": map[string]interface{}{
			"user_id": userID, "organization_id": orgID, "event_type": "*",
			"email_enabled": true, "in_app_enabled": true, "slack_enabled": false,
			"digest_frequency": "immediate",
		}})
		return
	}
	if err != nil {
		writeNotificationInternalError(w, r, "load notification preferences", "Failed to load preferences", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": p})
}

// UpdatePreferences handles PUT /notifications/preferences.
// Accepts a list of preference objects to upsert.
func (h *NotificationHandler) UpdatePreferences(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserIDFromContext(r.Context())
	orgID := middleware.GetOrgIDFromContext(r.Context())
	if userID == "" || orgID == "" {
		writeError(w, http.StatusUnauthorized, "Authentication required", "")
		return
	}

	var input struct {
		EmailEnabled       *bool   `json:"email_enabled"`
		InAppEnabled       *bool   `json:"in_app_enabled"`
		SlackEnabled       *bool   `json:"slack_enabled"`
		DigestFrequency    *string `json:"digest_frequency"`
		QuietHoursStart    *string `json:"quiet_hours_start"`
		QuietHoursEnd      *string `json:"quiet_hours_end"`
		QuietHoursTimezone *string `json:"quiet_hours_timezone"`
	}
	if err := decodeNotificationJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	if input.DigestFrequency != nil {
		switch *input.DigestFrequency {
		case "immediate", "hourly", "daily", "weekly":
		default:
			writeError(w, http.StatusBadRequest, "Invalid digest frequency", "")
			return
		}
	}
	for _, quietTime := range []*string{input.QuietHoursStart, input.QuietHoursEnd} {
		if quietTime != nil {
			if _, err := time.Parse("15:04", *quietTime); err != nil {
				writeError(w, http.StatusBadRequest, "Quiet hours must use HH:MM format", "")
				return
			}
		}
	}
	if input.QuietHoursTimezone != nil {
		if _, err := time.LoadLocation(*input.QuietHoursTimezone); err != nil {
			writeError(w, http.StatusBadRequest, "Invalid quiet-hours timezone", "")
			return
		}
	}

	var updated notificationPreference
	err := database.QuerierFromContext(r.Context(), h.pool).QueryRow(r.Context(),
		`INSERT INTO notification_preferences
			(user_id, organization_id, event_type, email_enabled, in_app_enabled, slack_enabled,
			 digest_frequency, quiet_hours_start, quiet_hours_end, quiet_hours_timezone,
			 created_at, updated_at)
		 VALUES ($1, $2, '*', COALESCE($3, true), COALESCE($4, true), COALESCE($5, false),
		         COALESCE($6, 'immediate')::digest_frequency, $7::time, $8::time, $9, NOW(), NOW())
		 ON CONFLICT (user_id, organization_id, event_type)
		 DO UPDATE SET
			email_enabled = COALESCE($3, notification_preferences.email_enabled),
			in_app_enabled = COALESCE($4, notification_preferences.in_app_enabled),
			slack_enabled = COALESCE($5, notification_preferences.slack_enabled),
			digest_frequency = COALESCE($6::digest_frequency, notification_preferences.digest_frequency),
			quiet_hours_start = COALESCE($7::time, notification_preferences.quiet_hours_start),
			quiet_hours_end = COALESCE($8::time, notification_preferences.quiet_hours_end),
			quiet_hours_timezone = COALESCE($9, notification_preferences.quiet_hours_timezone),
			updated_at = NOW()
		 RETURNING id,user_id,organization_id,event_type,email_enabled,in_app_enabled,slack_enabled,
		           digest_frequency,quiet_hours_start,quiet_hours_end,quiet_hours_timezone,created_at,updated_at`,
		userID, orgID, input.EmailEnabled, input.InAppEnabled, input.SlackEnabled,
		input.DigestFrequency, input.QuietHoursStart, input.QuietHoursEnd, input.QuietHoursTimezone,
	).Scan(&updated.ID, &updated.UserID, &updated.OrganizationID, &updated.EventType,
		&updated.EmailEnabled, &updated.InAppEnabled, &updated.SlackEnabled,
		&updated.DigestFrequency, &updated.QuietHoursStart, &updated.QuietHoursEnd,
		&updated.QuietHoursTimezone, &updated.CreatedAt, &updated.UpdatedAt)
	if err != nil {
		writeNotificationInternalError(w, r, "update notification preferences", "Failed to update preferences", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": updated})
}

// --------------------------------------------------------------------------
// Admin endpoints
// --------------------------------------------------------------------------

// ListRules handles GET /settings/notification-rules.
func (h *NotificationHandler) ListRules(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusUnauthorized, "Authentication required", "")
		return
	}

	pagination := parsePagination(r)
	offset := (pagination.Page - 1) * pagination.PageSize

	var total int
	err := database.QuerierFromContext(r.Context(), h.pool).QueryRow(r.Context(),
		`SELECT COUNT(*) FROM notification_rules WHERE organization_id = $1`,
		orgID,
	).Scan(&total)
	if err != nil {
		writeNotificationInternalError(w, r, "count notification rules", "Failed to count rules", err)
		return
	}

	rows, err := database.QuerierFromContext(r.Context(), h.pool).Query(r.Context(),
		`SELECT id, organization_id, name, event_type, severity_filter, conditions,
		        channel_ids, recipient_type, recipient_ids, template_id, is_active, cooldown_minutes,
		        created_at, updated_at
		 FROM notification_rules
		 WHERE organization_id = $1
		 ORDER BY created_at DESC
		 LIMIT $2 OFFSET $3`,
		orgID, pagination.PageSize, offset,
	)
	if err != nil {
		writeNotificationInternalError(w, r, "list notification rules", "Failed to list rules", err)
		return
	}
	defer rows.Close()

	rules := make([]notificationRuleResponse, 0)
	for rows.Next() {
		var rule notificationRuleResponse
		var conditionsJSON []byte
		if err := rows.Scan(
			&rule.ID, &rule.OrgID, &rule.Name, &rule.EventType,
			&rule.SeverityFilter, &conditionsJSON,
			&rule.ChannelIDs, &rule.RecipientType, &rule.RecipientIDs,
			&rule.TemplateID, &rule.IsActive, &rule.CooldownMinutes,
			&rule.CreatedAt, &rule.UpdatedAt,
		); err != nil {
			writeNotificationInternalError(w, r, "scan notification rule", "Failed to list rules", err)
			return
		}
		if len(conditionsJSON) > 0 {
			if err := json.Unmarshal(conditionsJSON, &rule.Conditions); err != nil {
				writeError(w, http.StatusInternalServerError, "Stored rule conditions are invalid", "")
				return
			}
		}
		rules = append(rules, rule)
	}

	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": rules,
		"pagination": models.PaginationResponse{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}

// CreateRule handles POST /settings/notification-rules.
func (h *NotificationHandler) CreateRule(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusUnauthorized, "Authentication required", "")
		return
	}

	var input notificationRuleInput
	if err := decodeNotificationJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	querier := database.QuerierFromContext(r.Context(), h.pool)
	if err := validateNotificationRuleInput(r.Context(), querier, orgID, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid notification rule", err.Error())
		return
	}
	conditionsJSON, err := json.Marshal(input.Conditions)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid rule conditions", "")
		return
	}
	isActive := true
	if input.IsActive != nil {
		isActive = *input.IsActive
	}

	var ruleID string
	err = querier.QueryRow(r.Context(),
		`INSERT INTO notification_rules
			(organization_id, name, event_type, severity_filter, conditions,
			 channel_ids, recipient_type, recipient_ids, template_id, is_active, cooldown_minutes,
			 created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW(), NOW())
		 RETURNING id`,
		orgID, input.Name, input.EventType, input.SeverityFilter, conditionsJSON,
		input.ChannelIDs, input.RecipientType, input.RecipientIDs,
		nullableString(input.TemplateID), isActive, input.CooldownMinutes,
	).Scan(&ruleID)
	if err != nil {
		writeNotificationInternalError(w, r, "create notification rule", "Failed to create rule", err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"id":      ruleID,
		"message": "Notification rule created",
	})
}

// UpdateRule handles PUT /settings/notification-rules/{id}.
func (h *NotificationHandler) UpdateRule(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusUnauthorized, "Authentication required", "")
		return
	}

	ruleID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(ruleID); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid rule ID", "")
		return
	}

	var input notificationRuleInput
	if err := decodeNotificationJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	querier := database.QuerierFromContext(r.Context(), h.pool)
	if err := validateNotificationRuleInput(r.Context(), querier, orgID, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid notification rule", err.Error())
		return
	}
	conditionsJSON, err := json.Marshal(input.Conditions)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid rule conditions", "")
		return
	}

	result, err := querier.Exec(r.Context(),
		`UPDATE notification_rules
		 SET name = $1, event_type = $2, severity_filter = $3, conditions = $4,
		     channel_ids = $5, recipient_type = $6, recipient_ids = $7,
		     template_id = $8, is_active = COALESCE($9, is_active), cooldown_minutes = $10, updated_at = NOW()
		 WHERE id = $11 AND organization_id = $12`,
		input.Name, input.EventType, input.SeverityFilter, conditionsJSON,
		input.ChannelIDs, input.RecipientType, input.RecipientIDs,
		nullableString(input.TemplateID), input.IsActive, input.CooldownMinutes,
		ruleID, orgID,
	)
	if err != nil {
		writeNotificationInternalError(w, r, "update notification rule", "Failed to update rule", err)
		return
	}

	if result.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "Rule not found", "")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Notification rule updated"})
}

// DeleteRule handles DELETE /settings/notification-rules/{id}.
func (h *NotificationHandler) DeleteRule(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusUnauthorized, "Authentication required", "")
		return
	}

	ruleID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(ruleID); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid rule ID", "")
		return
	}

	result, err := database.QuerierFromContext(r.Context(), h.pool).Exec(r.Context(),
		`UPDATE notification_rules SET is_active=false, updated_at=NOW()
		 WHERE id = $1 AND organization_id = $2`,
		ruleID, orgID,
	)
	if err != nil {
		writeNotificationInternalError(w, r, "delete notification rule", "Failed to delete rule", err)
		return
	}

	if result.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "Rule not found", "")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ListTemplates handles GET /settings/notification-templates and returns both
// immutable system templates and tenant-owned overrides.
func (h *NotificationHandler) ListTemplates(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusUnauthorized, "Authentication required", "")
		return
	}
	pagination := parsePagination(r)
	offset := (pagination.Page - 1) * pagination.PageSize
	querier := database.QuerierFromContext(r.Context(), h.pool)

	var total int
	if err := querier.QueryRow(r.Context(), `
		SELECT COUNT(*) FROM notification_templates
		WHERE organization_id IS NULL OR organization_id=$1`, orgID).Scan(&total); err != nil {
		writeNotificationInternalError(w, r, "count notification templates", "Failed to list templates", err)
		return
	}
	rows, err := querier.Query(r.Context(), `
		SELECT id, organization_id, name, event_type,
		       COALESCE(subject_template,''), COALESCE(body_html_template,''),
		       COALESCE(body_text_template,''), COALESCE(variables,'{}'::text[]),
		       is_system, created_at, updated_at
		FROM notification_templates
		WHERE organization_id IS NULL OR organization_id=$1
		ORDER BY is_system DESC, name ASC, created_at DESC
		LIMIT $2 OFFSET $3`, orgID, pagination.PageSize, offset)
	if err != nil {
		writeNotificationInternalError(w, r, "list notification templates", "Failed to list templates", err)
		return
	}
	defer rows.Close()
	templates := make([]notificationTemplateResponse, 0)
	for rows.Next() {
		var item notificationTemplateResponse
		if err := rows.Scan(
			&item.ID, &item.OrgID, &item.Name, &item.EventType,
			&item.SubjectTemplate, &item.BodyHTMLTemplate, &item.BodyTextTemplate,
			&item.Variables, &item.IsSystem, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			writeNotificationInternalError(w, r, "scan notification template", "Failed to list templates", err)
			return
		}
		templates = append(templates, item)
	}
	if err := rows.Err(); err != nil {
		writeNotificationInternalError(w, r, "iterate notification templates", "Failed to list templates", err)
		return
	}

	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": templates,
		"pagination": models.PaginationResponse{
			Page: pagination.Page, PageSize: pagination.PageSize,
			TotalItems: total, TotalPages: totalPages,
		},
	})
}

// CreateTemplate handles POST /settings/notification-templates.
func (h *NotificationHandler) CreateTemplate(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusUnauthorized, "Authentication required", "")
		return
	}
	var input notificationTemplateInput
	if err := decodeNotificationJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	if err := validateNotificationTemplateInput(&input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid notification template", err.Error())
		return
	}

	var templateID string
	err := database.QuerierFromContext(r.Context(), h.pool).QueryRow(r.Context(), `
		INSERT INTO notification_templates
			(organization_id,name,event_type,subject_template,body_html_template,
			 body_text_template,variables,is_system,created_at,updated_at)
		VALUES ($1,$2,$3,$4,NULLIF($5,''),NULLIF($6,''),$7,false,NOW(),NOW())
		ON CONFLICT (organization_id,event_type,name) DO NOTHING
		RETURNING id`, orgID, input.Name, input.EventType, input.SubjectTemplate,
		input.BodyHTMLTemplate, input.BodyTextTemplate, input.Variables).Scan(&templateID)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusConflict, "A template with this name and event type already exists", "")
		return
	}
	if err != nil {
		writeNotificationInternalError(w, r, "create notification template", "Failed to create template", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{
		"id": templateID, "message": "Notification template created",
	})
}

// UpdateTemplate handles PUT /settings/notification-templates/{id}. System
// templates are intentionally immutable through tenant APIs.
func (h *NotificationHandler) UpdateTemplate(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusUnauthorized, "Authentication required", "")
		return
	}
	templateID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(templateID); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid template ID", "")
		return
	}
	var input notificationTemplateInput
	if err := decodeNotificationJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	if err := validateNotificationTemplateInput(&input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid notification template", err.Error())
		return
	}
	querier := database.QuerierFromContext(r.Context(), h.pool)
	var duplicate bool
	if err := querier.QueryRow(r.Context(), `
		SELECT EXISTS(SELECT 1 FROM notification_templates
		 WHERE organization_id=$1 AND event_type=$2 AND name=$3 AND id<>$4)`,
		orgID, input.EventType, input.Name, templateID).Scan(&duplicate); err != nil {
		writeNotificationInternalError(w, r, "check notification template uniqueness", "Failed to update template", err)
		return
	}
	if duplicate {
		writeError(w, http.StatusConflict, "A template with this name and event type already exists", "")
		return
	}
	result, err := querier.Exec(r.Context(), `
		UPDATE notification_templates
		SET name=$1,event_type=$2,subject_template=$3,body_html_template=NULLIF($4,''),
		    body_text_template=NULLIF($5,''),variables=$6,updated_at=NOW()
		WHERE id=$7 AND organization_id=$8 AND is_system=false`,
		input.Name, input.EventType, input.SubjectTemplate, input.BodyHTMLTemplate,
		input.BodyTextTemplate, input.Variables, templateID, orgID)
	if err != nil {
		writeNotificationInternalError(w, r, "update notification template", "Failed to update template", err)
		return
	}
	if result.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "Template not found", "")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"id": templateID, "message": "Notification template updated",
	})
}

// DeleteTemplate handles DELETE /settings/notification-templates/{id}.
func (h *NotificationHandler) DeleteTemplate(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusUnauthorized, "Authentication required", "")
		return
	}
	templateID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(templateID); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid template ID", "")
		return
	}
	querier := database.QuerierFromContext(r.Context(), h.pool)
	var usedByRule bool
	if err := querier.QueryRow(r.Context(), `
		SELECT EXISTS(SELECT 1 FROM notification_rules
		 WHERE organization_id=$1 AND template_id=$2 AND is_active=true)`, orgID, templateID).Scan(&usedByRule); err != nil {
		writeNotificationInternalError(w, r, "check notification template references", "Failed to delete template", err)
		return
	}
	if usedByRule {
		writeError(w, http.StatusConflict, "Template is used by a notification rule", "")
		return
	}
	result, err := querier.Exec(r.Context(), `
		DELETE FROM notification_templates
		WHERE id=$1 AND organization_id=$2 AND is_system=false`, templateID, orgID)
	if err != nil {
		writeNotificationInternalError(w, r, "delete notification template", "Failed to delete template", err)
		return
	}
	if result.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "Template not found", "")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListChannels handles GET /settings/notification-channels.
func (h *NotificationHandler) ListChannels(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusUnauthorized, "Authentication required", "")
		return
	}

	rows, err := database.QuerierFromContext(r.Context(), h.pool).Query(r.Context(),
		`SELECT id, organization_id, name, channel_type, configuration, is_active, created_at, updated_at
		 FROM notification_channels
		 WHERE organization_id = $1 AND deleted_at IS NULL
		 ORDER BY created_at DESC`,
		orgID,
	)
	if err != nil {
		writeNotificationInternalError(w, r, "list notification channels", "Failed to list channels", err)
		return
	}
	defer rows.Close()

	channels := make([]notificationChannelResponse, 0)
	for rows.Next() {
		var ch notificationChannelResponse
		var rawConfig []byte
		if err := rows.Scan(
			&ch.ID, &ch.OrgID, &ch.Name, &ch.ChannelType,
			&rawConfig, &ch.IsActive, &ch.CreatedAt, &ch.UpdatedAt,
		); err != nil {
			writeNotificationInternalError(w, r, "scan notification channel", "Failed to list channels", err)
			return
		}
		config, openErr := openHandlerNotificationChannelConfig(
			h.protector, ch.OrgID, ch.ID, ch.ChannelType, rawConfig,
		)
		if openErr != nil {
			err = openErr
		} else {
			ch.Config, err = redactNotificationChannelConfig(ch.ChannelType, config)
		}
		if err != nil {
			log.Error().Err(err).Str("channel_id", ch.ID).Msg("failed to redact notification channel")
			writeError(w, http.StatusInternalServerError, "Stored channel configuration is invalid", "")
			return
		}
		channels = append(channels, ch)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to iterate channels", "")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": channels})
}

// CreateChannel handles POST /settings/notification-channels.
func (h *NotificationHandler) CreateChannel(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusUnauthorized, "Authentication required", "")
		return
	}

	var input notificationChannelInput
	if err := decodeNotificationJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	if err := validateNotificationChannelInput(&input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid notification channel", err.Error())
		return
	}
	if h.protector == nil {
		writeError(w, http.StatusServiceUnavailable, "Notification secret protection is unavailable", "")
		return
	}
	channelID := uuid.NewString()
	configJSON, err := h.protector.SealJSON(
		orgID, notificationChannelConfigPurpose(channelID), input.Config,
	)
	if err != nil {
		log.Error().Err(err).Msg("failed to protect notification channel configuration")
		writeError(w, http.StatusInternalServerError, "Failed to protect channel configuration", "")
		return
	}
	isActive := true
	if input.IsActive != nil {
		isActive = *input.IsActive
	}

	err = database.QuerierFromContext(r.Context(), h.pool).QueryRow(r.Context(),
		`INSERT INTO notification_channels
			(id, organization_id, name, channel_type, configuration, is_active, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
		 RETURNING id`,
		channelID, orgID, input.Name, input.ChannelType, configJSON, isActive,
	).Scan(&channelID)
	if err != nil {
		writeNotificationInternalError(w, r, "create notification channel", "Failed to create channel", err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"id":      channelID,
		"message": "Notification channel created",
	})
}

// UpdateChannel handles PUT /settings/notification-channels/{id}. Channel
// configuration is replaced atomically and re-encrypted with tenant- and
// resource-bound associated data.
func (h *NotificationHandler) UpdateChannel(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusUnauthorized, "Authentication required", "")
		return
	}
	channelID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(channelID); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid channel ID", "")
		return
	}

	var input notificationChannelInput
	if err := decodeNotificationJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	if err := validateNotificationChannelInput(&input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid notification channel", err.Error())
		return
	}
	if h.protector == nil {
		writeError(w, http.StatusServiceUnavailable, "Notification secret protection is unavailable", "")
		return
	}
	configJSON, err := h.protector.SealJSON(
		orgID, notificationChannelConfigPurpose(channelID), input.Config,
	)
	if err != nil {
		writeNotificationInternalError(w, r, "protect notification channel", "Failed to protect channel configuration", err)
		return
	}
	isActive := true
	if input.IsActive != nil {
		isActive = *input.IsActive
	}

	result, err := database.QuerierFromContext(r.Context(), h.pool).Exec(r.Context(), `
		UPDATE notification_channels
		SET name=$1, channel_type=$2, configuration=$3, is_active=$4, updated_at=NOW()
		WHERE id=$5 AND organization_id=$6 AND deleted_at IS NULL`,
		input.Name, input.ChannelType, configJSON, isActive, channelID, orgID,
	)
	if err != nil {
		writeNotificationInternalError(w, r, "update notification channel", "Failed to update channel", err)
		return
	}
	if result.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "Channel not found", "")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"id":      channelID,
		"message": "Notification channel updated",
	})
}

// DeleteChannel handles DELETE /settings/notification-channels/{id}. A
// channel used by an active routing rule must first be removed from that rule,
// avoiding silent loss of mandatory notifications.
func (h *NotificationHandler) DeleteChannel(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusUnauthorized, "Authentication required", "")
		return
	}
	channelID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(channelID); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid channel ID", "")
		return
	}
	querier := database.QuerierFromContext(r.Context(), h.pool)

	var usedByActiveRule bool
	if err := querier.QueryRow(r.Context(), `
		SELECT EXISTS(
			SELECT 1 FROM notification_rules
			WHERE organization_id=$1 AND is_active=true
			  AND $2::uuid = ANY(COALESCE(channel_ids, '{}'::uuid[]))
		)`, orgID, channelID).Scan(&usedByActiveRule); err != nil {
		writeNotificationInternalError(w, r, "check notification channel references", "Failed to delete channel", err)
		return
	}
	if usedByActiveRule {
		writeError(w, http.StatusConflict, "Channel is used by an active notification rule", "")
		return
	}

	result, err := querier.Exec(r.Context(), `
		UPDATE notification_channels
		SET is_active=false, deleted_at=NOW(), configuration='{}'::jsonb, updated_at=NOW()
		WHERE id=$1 AND organization_id=$2 AND deleted_at IS NULL`, channelID, orgID)
	if err != nil {
		writeNotificationInternalError(w, r, "delete notification channel", "Failed to delete channel", err)
		return
	}
	if result.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "Channel not found", "")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// TestChannel handles POST /settings/notification-channels/{id}/test.
// Sends a test notification through the specified channel.
func (h *NotificationHandler) TestChannel(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	if orgID == "" || userID == "" {
		writeError(w, http.StatusUnauthorized, "Authentication required", "")
		return
	}

	channelID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(channelID); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid channel ID", "")
		return
	}

	// Load the channel configuration.
	var channelType string
	var configJSON []byte
	err := database.QuerierFromContext(r.Context(), h.pool).QueryRow(r.Context(),
		`SELECT channel_type, configuration FROM notification_channels
		 WHERE id = $1 AND organization_id = $2 AND is_active = true AND deleted_at IS NULL`,
		channelID, orgID,
	).Scan(&channelType, &configJSON)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Channel not found", "")
		return
	}
	if err != nil {
		writeNotificationInternalError(w, r, "load notification channel", "Failed to load channel", err)
		return
	}

	channelConfig, err := openHandlerNotificationChannelConfig(
		h.protector, orgID, channelID, channelType, configJSON,
	)
	if err != nil {
		log.Error().Err(err).Str("channel_id", channelID).Msg("failed to open notification channel configuration")
		writeError(w, http.StatusInternalServerError, "Channel configuration is unavailable", "")
		return
	}

	testNotification := service.Notification{
		ID:              "test-" + channelID,
		OrgID:           orgID,
		EventType:       "system.channel_test",
		RecipientUserID: userID,
		ChannelType:     channelType,
		Subject:         "[ComplianceForge] Test Notification",
		Body:            "This is a test notification to verify your notification channel configuration is working correctly.",
		Status:          "pending",
		CreatedAt:       time.Now().UTC(),
	}

	if h.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "Notification delivery is unavailable", "")
		return
	}
	if err := h.engine.Dispatch(r.Context(), testNotification, channelConfig); err != nil {
		log.Error().Err(err).Str("channel_id", channelID).Msg("notification channel test failed")
		writeError(w, http.StatusBadGateway, "Channel test failed", "")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"message":      "Test notification sent successfully",
		"channel_type": channelType,
	})
}

func decodeNotificationJSON(w http.ResponseWriter, r *http.Request, destination any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("request body must contain one JSON object")
		}
		return err
	}
	return nil
}

// writeNotificationInternalError keeps database, network, and cryptographic
// details out of API responses while retaining a request-correlated server log.
func writeNotificationInternalError(w http.ResponseWriter, r *http.Request, operation, message string, err error) {
	log.Error().
		Err(err).
		Str("operation", operation).
		Str("request_id", middleware.GetRequestIDFromContext(r.Context())).
		Msg("notification request failed")
	writeError(w, http.StatusInternalServerError, message, "")
}

func validateNotificationRuleInput(ctx context.Context, querier database.Querier, orgID string, input *notificationRuleInput) error {
	input.Name = strings.TrimSpace(input.Name)
	input.EventType = strings.ToLower(strings.TrimSpace(input.EventType))
	input.RecipientType = strings.ToLower(strings.TrimSpace(input.RecipientType))
	if input.Name == "" || len([]rune(input.Name)) > 200 {
		return fmt.Errorf("name is required and must not exceed 200 characters")
	}
	if input.EventType != "*" && !validNotificationToken(input.EventType, 100) {
		return fmt.Errorf("event_type must be '*' or a lowercase event token")
	}
	if input.CooldownMinutes < 0 || input.CooldownMinutes > 525600 {
		return fmt.Errorf("cooldown_minutes must be between 0 and 525600")
	}

	seenSeverities := make(map[string]struct{}, len(input.SeverityFilter))
	for index, severity := range input.SeverityFilter {
		severity = strings.ToLower(strings.TrimSpace(severity))
		switch severity {
		case "low", "medium", "high", "critical":
		default:
			return fmt.Errorf("severity_filter contains an unsupported severity")
		}
		if _, duplicate := seenSeverities[severity]; duplicate {
			return fmt.Errorf("severity_filter contains a duplicate")
		}
		seenSeverities[severity] = struct{}{}
		input.SeverityFilter[index] = severity
	}
	if len(input.Conditions) > 25 {
		return fmt.Errorf("conditions cannot contain more than 25 top-level entries")
	}
	for key := range input.Conditions {
		if !validNotificationToken(key, 100) {
			return fmt.Errorf("condition keys must be lowercase event tokens")
		}
	}
	conditionsJSON, err := json.Marshal(input.Conditions)
	if err != nil || len(conditionsJSON) > 64*1024 {
		return fmt.Errorf("conditions must be valid JSON no larger than 64 KiB")
	}

	input.ChannelIDs, err = uniqueNotificationUUIDs(input.ChannelIDs, 20, "channel_ids")
	if err != nil || len(input.ChannelIDs) == 0 {
		if err != nil {
			return err
		}
		return fmt.Errorf("at least one channel_id is required")
	}
	var channelCount int
	if err := querier.QueryRow(ctx, `
		SELECT COUNT(*) FROM notification_channels
		WHERE organization_id=$1 AND id=ANY($2::uuid[]) AND is_active=true AND deleted_at IS NULL`,
		orgID, input.ChannelIDs).Scan(&channelCount); err != nil {
		return fmt.Errorf("validate notification channels: %w", err)
	}
	if channelCount != len(input.ChannelIDs) {
		return fmt.Errorf("one or more channels are unavailable to this organization")
	}

	switch input.RecipientType {
	case "user", "custom", "role":
		input.RecipientIDs, err = uniqueNotificationUUIDs(input.RecipientIDs, 100, "recipient_ids")
		if err != nil {
			return err
		}
		if len(input.RecipientIDs) == 0 {
			return fmt.Errorf("recipient_ids is required for %s recipients", input.RecipientType)
		}
		var recipientCount int
		if input.RecipientType == "role" {
			err = querier.QueryRow(ctx, `
				SELECT COUNT(*) FROM roles
				WHERE id=ANY($1::uuid[]) AND (organization_id IS NULL OR organization_id=$2) AND deleted_at IS NULL`,
				input.RecipientIDs, orgID).Scan(&recipientCount)
		} else {
			err = querier.QueryRow(ctx, `
				SELECT COUNT(*) FROM users
				WHERE id=ANY($1::uuid[]) AND organization_id=$2 AND status='active' AND deleted_at IS NULL`,
				input.RecipientIDs, orgID).Scan(&recipientCount)
		}
		if err != nil {
			return fmt.Errorf("validate notification recipients: %w", err)
		}
		if recipientCount != len(input.RecipientIDs) {
			return fmt.Errorf("one or more recipients are unavailable to this organization")
		}
	case "owner", "assignee", "dpo", "ciso":
		if len(input.RecipientIDs) != 0 {
			return fmt.Errorf("recipient_ids must be empty for %s recipients", input.RecipientType)
		}
	default:
		return fmt.Errorf("recipient_type is unsupported")
	}

	if input.TemplateID != nil {
		trimmed := strings.TrimSpace(*input.TemplateID)
		if trimmed == "" {
			input.TemplateID = nil
		} else {
			if _, err := uuid.Parse(trimmed); err != nil {
				return fmt.Errorf("template_id is invalid")
			}
			var exists bool
			if err := querier.QueryRow(ctx, `
				SELECT EXISTS(SELECT 1 FROM notification_templates
				WHERE id=$1 AND (organization_id IS NULL OR organization_id=$2))`, trimmed, orgID).Scan(&exists); err != nil {
				return fmt.Errorf("validate notification template: %w", err)
			}
			if !exists {
				return fmt.Errorf("template is unavailable to this organization")
			}
			input.TemplateID = &trimmed
		}
	}
	return nil
}

func validateNotificationChannelInput(input *notificationChannelInput) error {
	input.Name = strings.TrimSpace(input.Name)
	input.ChannelType = strings.ToLower(strings.TrimSpace(input.ChannelType))
	if input.Name == "" || len([]rune(input.Name)) > 200 {
		return fmt.Errorf("name is required and must not exceed 200 characters")
	}
	if input.Config == nil {
		input.Config = make(map[string]any)
	}
	switch input.ChannelType {
	case "email", "in_app":
		if len(input.Config) != 0 {
			return fmt.Errorf("%s channels do not accept per-channel credentials", input.ChannelType)
		}
	case "webhook":
		if err := requireOnlyChannelKeys(input.Config, "url", "secret"); err != nil {
			return err
		}
		rawURL, err := notificationConfigString(input.Config, "url")
		if err != nil {
			return err
		}
		if len(rawURL) > 2048 {
			return fmt.Errorf("webhook URL is too long")
		}
		destination, err := url.Parse(rawURL)
		if err != nil || safehttp.ValidateURL(destination, safehttp.Policy{}) != nil {
			return fmt.Errorf("webhook URL must be a public HTTPS endpoint")
		}
		secret, err := notificationConfigString(input.Config, "secret")
		if err != nil {
			return err
		}
		if len(secret) < 32 || len(secret) > 512 {
			return fmt.Errorf("webhook secret must contain between 32 and 512 characters")
		}
	case "slack":
		if err := requireOnlyChannelKeys(input.Config, "webhook_url"); err != nil {
			return err
		}
		rawURL, err := notificationConfigString(input.Config, "webhook_url")
		if err != nil {
			return err
		}
		destination, err := url.Parse(rawURL)
		if err != nil || safehttp.ValidateURL(destination, safehttp.Policy{AllowedHosts: []string{"hooks.slack.com"}}) != nil {
			return fmt.Errorf("Slack webhook URL must use hooks.slack.com over HTTPS")
		}
	default:
		return fmt.Errorf("channel_type must be email, in_app, webhook, or slack")
	}
	return nil
}

func validateNotificationTemplateInput(input *notificationTemplateInput) error {
	input.Name = strings.TrimSpace(input.Name)
	input.EventType = strings.ToLower(strings.TrimSpace(input.EventType))
	if input.Name == "" || len([]rune(input.Name)) > 200 {
		return fmt.Errorf("name is required and must not exceed 200 characters")
	}
	if !validNotificationToken(input.EventType, 100) {
		return fmt.Errorf("event_type must be a lowercase event token")
	}
	variables := make([]string, 0, len(input.Variables))
	seen := make(map[string]struct{}, len(input.Variables))
	if len(input.Variables) > 100 {
		return fmt.Errorf("variables cannot contain more than 100 entries")
	}
	for _, variable := range input.Variables {
		variable = strings.ToLower(strings.TrimSpace(variable))
		if !validNotificationToken(variable, 100) {
			return fmt.Errorf("variables must contain lowercase event tokens")
		}
		if _, exists := seen[variable]; exists {
			return fmt.Errorf("variables contains a duplicate")
		}
		seen[variable] = struct{}{}
		variables = append(variables, variable)
	}
	input.Variables = variables
	return service.ValidateNotificationTemplateDefinition(
		input.SubjectTemplate, input.BodyTextTemplate, input.BodyHTMLTemplate,
	)
}

func redactNotificationChannelConfig(channelType string, config map[string]any) (map[string]any, error) {
	switch channelType {
	case "email":
		return map[string]any{"transport": "platform"}, nil
	case "in_app":
		return map[string]any{}, nil
	case "webhook":
		response := map[string]any{"secret_configured": false}
		if secret, ok := config["secret"].(string); ok && secret != "" {
			response["secret_configured"] = true
		}
		if rawURL, ok := config["url"].(string); ok {
			if parsed, err := url.Parse(rawURL); err == nil && parsed.Host != "" {
				response["endpoint_origin"] = parsed.Scheme + "://" + parsed.Host
			}
		}
		return response, nil
	case "slack":
		webhook, _ := config["webhook_url"].(string)
		return map[string]any{"webhook_configured": webhook != ""}, nil
	default:
		return nil, fmt.Errorf("unsupported channel type")
	}
}

func uniqueNotificationUUIDs(values []string, maximum int, field string) ([]string, error) {
	if len(values) > maximum {
		return nil, fmt.Errorf("%s cannot contain more than %d entries", field, maximum)
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		parsed, err := uuid.Parse(value)
		if err != nil {
			return nil, fmt.Errorf("%s contains an invalid UUID", field)
		}
		canonical := parsed.String()
		if _, duplicate := seen[canonical]; duplicate {
			return nil, fmt.Errorf("%s contains a duplicate UUID", field)
		}
		seen[canonical] = struct{}{}
		result = append(result, canonical)
	}
	return result, nil
}

func validNotificationToken(value string, maximum int) bool {
	if value == "" || len(value) > maximum {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') ||
			character == '.' || character == '_' || character == '-' {
			continue
		}
		return false
	}
	return true
}

func nullableString(value *string) any {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil
	}
	return strings.TrimSpace(*value)
}

func requireOnlyChannelKeys(config map[string]any, allowed ...string) error {
	for key := range config {
		matched := false
		for _, candidate := range allowed {
			if key == candidate {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("channel config contains unsupported field %q", key)
		}
	}
	return nil
}

func notificationConfigString(config map[string]any, key string) (string, error) {
	value, ok := config[key]
	if !ok {
		return "", fmt.Errorf("channel config field %q is required", key)
	}
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("channel config field %q must be a non-empty string", key)
	}
	return strings.TrimSpace(text), nil
}

func openHandlerNotificationChannelConfig(
	protector *secretbox.Box,
	orgID, channelID, channelType string,
	document []byte,
) (map[string]any, error) {
	config := make(map[string]any)
	if protector != nil {
		if err := protector.OpenJSON(orgID, notificationChannelConfigPurpose(channelID), document, &config); err == nil {
			return config, nil
		}
	}
	if channelType == "email" || channelType == "in_app" {
		var legacy map[string]any
		if len(document) == 0 {
			return config, nil
		}
		if err := json.Unmarshal(document, &legacy); err == nil && len(legacy) == 0 {
			return config, nil
		}
	}
	return nil, fmt.Errorf("encrypted notification channel configuration is unavailable")
}

func notificationChannelConfigPurpose(channelID string) string {
	return "notification-channel/" + channelID
}

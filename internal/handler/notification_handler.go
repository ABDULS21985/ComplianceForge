package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"

	"github.com/complianceforge/platform/internal/middleware"
	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/service"
)

// NotificationHandler handles notification-related API endpoints.
type NotificationHandler struct {
	pool   *pgxpool.Pool
	engine *service.NotificationEngine
}

// NewNotificationHandler creates a new NotificationHandler.
func NewNotificationHandler(pool *pgxpool.Pool, engine *service.NotificationEngine) *NotificationHandler {
	return &NotificationHandler{
		pool:   pool,
		engine: engine,
	}
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
	err := h.pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM notifications
		 WHERE recipient_user_id = $1 AND organization_id = $2 AND channel_type = 'in_app'`,
		userID, orgID,
	).Scan(&total)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to count notifications", err.Error())
		return
	}

	// Fetch paginated notifications, newest first.
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, organization_id, event_type, recipient_user_id, channel_type,
		        subject, body, status, created_at, read_at
		 FROM notifications
		 WHERE recipient_user_id = $1 AND organization_id = $2 AND channel_type = 'in_app'
		 ORDER BY created_at DESC
		 LIMIT $3 OFFSET $4`,
		userID, orgID, pagination.PageSize, offset,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list notifications", err.Error())
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
			writeError(w, http.StatusInternalServerError, "Failed to scan notification", err.Error())
			return
		}
		notifications = append(notifications, n)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to iterate notifications", err.Error())
		return
	}

	// Fetch unread count and include in header.
	var unreadCount int
	err = h.pool.QueryRow(r.Context(),
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
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Authentication required", "")
		return
	}

	notifID := chi.URLParam(r, "id")
	if notifID == "" {
		writeError(w, http.StatusBadRequest, "Missing notification ID", "")
		return
	}

	now := time.Now().UTC()
	result, err := h.pool.Exec(r.Context(),
		`UPDATE notifications SET read_at = $1, updated_at = $1
		 WHERE id = $2 AND recipient_user_id = $3 AND read_at IS NULL`,
		now, notifID, userID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to mark notification as read", err.Error())
		return
	}

	if result.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "Notification not found or already read", "")
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

	now := time.Now().UTC()
	result, err := h.pool.Exec(r.Context(),
		`UPDATE notifications SET read_at = $1, updated_at = $1
		 WHERE recipient_user_id = $2 AND organization_id = $3
		   AND channel_type = 'in_app' AND read_at IS NULL`,
		now, userID, orgID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to mark all notifications as read", err.Error())
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
	err := h.pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM notifications
		 WHERE recipient_user_id = $1 AND organization_id = $2
		   AND channel_type = 'in_app' AND read_at IS NULL`,
		userID, orgID,
	).Scan(&count)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to count unread notifications", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]int{"count": count})
}

// GetPreferences handles GET /notifications/preferences.
func (h *NotificationHandler) GetPreferences(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Authentication required", "")
		return
	}

	type Preference struct {
		ID                 string    `json:"id"`
		UserID             string    `json:"user_id"`
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

	var p Preference
	err := h.pool.QueryRow(r.Context(),
		`SELECT id, user_id, email_enabled, in_app_enabled, slack_enabled,
		        digest_frequency, quiet_hours_start, quiet_hours_end, quiet_hours_timezone,
		        created_at, updated_at
		 FROM notification_preferences
		 WHERE user_id = $1`,
		userID,
	).Scan(&p.ID, &p.UserID, &p.EmailEnabled, &p.InAppEnabled, &p.SlackEnabled,
		&p.DigestFrequency, &p.QuietHoursStart, &p.QuietHoursEnd, &p.QuietHoursTimezone,
		&p.CreatedAt, &p.UpdatedAt)
	if err == pgx.ErrNoRows {
		writeJSON(w, http.StatusOK, map[string]interface{}{"data": nil})
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to load preferences", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": p})
}

// UpdatePreferences handles PUT /notifications/preferences.
// Accepts a list of preference objects to upsert.
func (h *NotificationHandler) UpdatePreferences(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserIDFromContext(r.Context())
	if userID == "" {
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
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	_, err := h.pool.Exec(r.Context(),
		`INSERT INTO notification_preferences
			(user_id, email_enabled, in_app_enabled, slack_enabled,
			 digest_frequency, quiet_hours_start, quiet_hours_end, quiet_hours_timezone,
			 created_at, updated_at)
		 VALUES ($1, COALESCE($2, true), COALESCE($3, true), COALESCE($4, true),
		         COALESCE($5, 'none'), $6, $7, $8, NOW(), NOW())
		 ON CONFLICT (user_id)
		 DO UPDATE SET
			email_enabled = COALESCE($2, notification_preferences.email_enabled),
			in_app_enabled = COALESCE($3, notification_preferences.in_app_enabled),
			slack_enabled = COALESCE($4, notification_preferences.slack_enabled),
			digest_frequency = COALESCE($5, notification_preferences.digest_frequency),
			quiet_hours_start = COALESCE($6, notification_preferences.quiet_hours_start),
			quiet_hours_end = COALESCE($7, notification_preferences.quiet_hours_end),
			quiet_hours_timezone = COALESCE($8, notification_preferences.quiet_hours_timezone),
			updated_at = NOW()`,
		userID, input.EmailEnabled, input.InAppEnabled, input.SlackEnabled,
		input.DigestFrequency, input.QuietHoursStart, input.QuietHoursEnd, input.QuietHoursTimezone,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update preferences", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Preferences updated"})
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
	err := h.pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM notification_rules WHERE organization_id = $1`,
		orgID,
	).Scan(&total)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to count rules", err.Error())
		return
	}

	rows, err := h.pool.Query(r.Context(),
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
		writeError(w, http.StatusInternalServerError, "Failed to list rules", err.Error())
		return
	}
	defer rows.Close()

	type RuleResponse struct {
		ID              string                 `json:"id"`
		OrgID           string                 `json:"organization_id"`
		Name            string                 `json:"name"`
		EventType       string                 `json:"event_type"`
		SeverityFilter  json.RawMessage        `json:"severity_filter"`
		Conditions      json.RawMessage        `json:"conditions"`
		ChannelIDs      json.RawMessage        `json:"channel_ids"`
		RecipientType   string                 `json:"recipient_type"`
		RecipientIDs    json.RawMessage        `json:"recipient_ids"`
		TemplateID      string                 `json:"template_id"`
		IsActive        bool                   `json:"is_active"`
		CooldownMinutes int                    `json:"cooldown_minutes"`
		CreatedAt       time.Time              `json:"created_at"`
		UpdatedAt       time.Time              `json:"updated_at"`
	}

	rules := make([]RuleResponse, 0)
	for rows.Next() {
		var rule RuleResponse
		if err := rows.Scan(
			&rule.ID, &rule.OrgID, &rule.Name, &rule.EventType,
			&rule.SeverityFilter, &rule.Conditions,
			&rule.ChannelIDs, &rule.RecipientType, &rule.RecipientIDs,
			&rule.TemplateID, &rule.IsActive, &rule.CooldownMinutes,
			&rule.CreatedAt, &rule.UpdatedAt,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to scan rule", err.Error())
			return
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

	var input struct {
		Name            string                 `json:"name"`
		EventType       string                 `json:"event_type"`
		SeverityFilter  []string               `json:"severity_filter"`
		Conditions      map[string]interface{} `json:"conditions"`
		ChannelIDs      []string               `json:"channel_ids"`
		RecipientType   string                 `json:"recipient_type"`
		RecipientIDs    []string               `json:"recipient_ids"`
		TemplateID      string                 `json:"template_id"`
		IsActive        bool                   `json:"is_active"`
		CooldownMinutes int                    `json:"cooldown_minutes"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if input.Name == "" || input.EventType == "" || input.RecipientType == "" {
		writeError(w, http.StatusBadRequest, "name, event_type, and recipient_type are required", "")
		return
	}

	severityJSON, _ := json.Marshal(input.SeverityFilter)
	conditionsJSON, _ := json.Marshal(input.Conditions)
	channelIDsJSON, _ := json.Marshal(input.ChannelIDs)
	recipientIDsJSON, _ := json.Marshal(input.RecipientIDs)

	var ruleID string
	err := h.pool.QueryRow(r.Context(),
		`INSERT INTO notification_rules
			(organization_id, name, event_type, severity_filter, conditions,
			 channel_ids, recipient_type, recipient_ids, template_id, is_active, cooldown_minutes,
			 created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW(), NOW())
		 RETURNING id`,
		orgID, input.Name, input.EventType, severityJSON, conditionsJSON,
		channelIDsJSON, input.RecipientType, recipientIDsJSON,
		input.TemplateID, input.IsActive, input.CooldownMinutes,
	).Scan(&ruleID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create rule", err.Error())
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
	if ruleID == "" {
		writeError(w, http.StatusBadRequest, "Missing rule ID", "")
		return
	}

	var input struct {
		Name            string                 `json:"name"`
		EventType       string                 `json:"event_type"`
		SeverityFilter  []string               `json:"severity_filter"`
		Conditions      map[string]interface{} `json:"conditions"`
		ChannelIDs      []string               `json:"channel_ids"`
		RecipientType   string                 `json:"recipient_type"`
		RecipientIDs    []string               `json:"recipient_ids"`
		TemplateID      string                 `json:"template_id"`
		IsActive        bool                   `json:"is_active"`
		CooldownMinutes int                    `json:"cooldown_minutes"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	severityJSON, _ := json.Marshal(input.SeverityFilter)
	conditionsJSON, _ := json.Marshal(input.Conditions)
	channelIDsJSON, _ := json.Marshal(input.ChannelIDs)
	recipientIDsJSON, _ := json.Marshal(input.RecipientIDs)

	result, err := h.pool.Exec(r.Context(),
		`UPDATE notification_rules
		 SET name = $1, event_type = $2, severity_filter = $3, conditions = $4,
		     channel_ids = $5, recipient_type = $6, recipient_ids = $7,
		     template_id = $8, is_active = $9, cooldown_minutes = $10, updated_at = NOW()
		 WHERE id = $11 AND organization_id = $12`,
		input.Name, input.EventType, severityJSON, conditionsJSON,
		channelIDsJSON, input.RecipientType, recipientIDsJSON,
		input.TemplateID, input.IsActive, input.CooldownMinutes,
		ruleID, orgID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update rule", err.Error())
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
	if ruleID == "" {
		writeError(w, http.StatusBadRequest, "Missing rule ID", "")
		return
	}

	result, err := h.pool.Exec(r.Context(),
		`DELETE FROM notification_rules WHERE id = $1 AND organization_id = $2`,
		ruleID, orgID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to delete rule", err.Error())
		return
	}

	if result.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "Rule not found", "")
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

	rows, err := h.pool.Query(r.Context(),
		`SELECT id, organization_id, name, channel_type, configuration, is_active, created_at, updated_at
		 FROM notification_channels
		 WHERE organization_id = $1
		 ORDER BY created_at DESC`,
		orgID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list channels", err.Error())
		return
	}
	defer rows.Close()

	type ChannelResponse struct {
		ID          string          `json:"id"`
		OrgID       string          `json:"organization_id"`
		Name        string          `json:"name"`
		ChannelType string          `json:"channel_type"`
		Config      json.RawMessage `json:"config"`
		IsActive    bool            `json:"is_active"`
		CreatedAt   time.Time       `json:"created_at"`
		UpdatedAt   time.Time       `json:"updated_at"`
	}

	channels := make([]ChannelResponse, 0)
	for rows.Next() {
		var ch ChannelResponse
		if err := rows.Scan(
			&ch.ID, &ch.OrgID, &ch.Name, &ch.ChannelType,
			&ch.Config, &ch.IsActive, &ch.CreatedAt, &ch.UpdatedAt,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to scan channel", err.Error())
			return
		}
		channels = append(channels, ch)
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

	var input struct {
		Name        string            `json:"name"`
		ChannelType string            `json:"channel_type"`
		Config      map[string]string `json:"config"`
		IsActive    bool              `json:"is_active"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if input.Name == "" || input.ChannelType == "" {
		writeError(w, http.StatusBadRequest, "name and channel_type are required", "")
		return
	}

	validTypes := map[string]bool{"email": true, "in_app": true, "webhook": true, "slack": true}
	if !validTypes[input.ChannelType] {
		writeError(w, http.StatusBadRequest,
			fmt.Sprintf("Invalid channel_type: %s. Must be one of: email, in_app, webhook, slack", input.ChannelType), "")
		return
	}

	configJSON, _ := json.Marshal(input.Config)

	var channelID string
	err := h.pool.QueryRow(r.Context(),
		`INSERT INTO notification_channels
			(organization_id, name, channel_type, configuration, is_active, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
		 RETURNING id`,
		orgID, input.Name, input.ChannelType, configJSON, input.IsActive,
	).Scan(&channelID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create channel", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"id":      channelID,
		"message": "Notification channel created",
	})
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
	if channelID == "" {
		writeError(w, http.StatusBadRequest, "Missing channel ID", "")
		return
	}

	// Load the channel configuration.
	var channelType string
	var configJSON []byte
	err := h.pool.QueryRow(r.Context(),
		`SELECT channel_type, configuration FROM notification_channels
		 WHERE id = $1 AND organization_id = $2`,
		channelID, orgID,
	).Scan(&channelType, &configJSON)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Channel not found", "")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to load channel", err.Error())
		return
	}

	channelConfig := make(map[string]string)
	if configJSON != nil {
		json.Unmarshal(configJSON, &channelConfig)
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

	if err := h.engine.Dispatch(r.Context(), testNotification, channelConfig); err != nil {
		writeError(w, http.StatusInternalServerError, "Channel test failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"message":      "Test notification sent successfully",
		"channel_type": channelType,
	})
}

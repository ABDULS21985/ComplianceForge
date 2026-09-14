package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/complianceforge/platform/internal/middleware"
	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/service"
)

// IntegrationSvc defines the methods required by IntegrationHandler.
type IntegrationSvc interface {
	ListIntegrations(ctx context.Context, orgID string) ([]service.Integration, error)
	CreateIntegration(ctx context.Context, orgID, userID string, integ service.Integration, configJSON string) (*service.Integration, error)
	GetIntegration(ctx context.Context, orgID, integID string) (*service.Integration, error)
	UpdateIntegration(ctx context.Context, orgID, integID string, integ service.Integration, configJSON *string) error
	DeleteIntegration(ctx context.Context, orgID, integID string) error
	TestConnection(ctx context.Context, orgID, integID string) (string, error)
	TriggerSync(ctx context.Context, orgID, integID, syncType string) (*service.SyncLog, error)
	GetSyncLogs(ctx context.Context, orgID, integID string, page, pageSize int) ([]service.SyncLog, int, error)
	GetSSOConfig(ctx context.Context, orgID string) (*service.SSOConfiguration, error)
	UpdateSSOConfig(ctx context.Context, orgID string, config service.UpdateSSOConfigurationInput) error
	ListAPIKeys(ctx context.Context, orgID string) ([]service.APIKey, error)
	CreateAPIKey(ctx context.Context, orgID, userID, name string, permissions []string, rateLimit int, expiresAt *time.Time) (*service.APIKey, string, error)
	RevokeAPIKey(ctx context.Context, orgID, keyID string) error
}

var _ IntegrationSvc = (*service.IntegrationService)(nil)

// IntegrationHandler handles integration management endpoints.
type IntegrationHandler struct {
	svc IntegrationSvc
}

// NewIntegrationHandler creates a new IntegrationHandler with the given service.
func NewIntegrationHandler(svc IntegrationSvc) *IntegrationHandler {
	return &IntegrationHandler{svc: svc}
}

func (h *IntegrationHandler) Ready() bool { return h != nil && h.svc != nil }

type integrationPayload struct {
	Integration     *service.Integration `json:"integration,omitempty"`
	IntegrationType string               `json:"integration_type,omitempty"`
	Type            string               `json:"type,omitempty"`
	Name            string               `json:"name,omitempty"`
	Description     *string              `json:"description,omitempty"`
	SyncFrequency   int                  `json:"sync_frequency_minutes,omitempty"`
	Capabilities    []string             `json:"capabilities,omitempty"`
	Configuration   json.RawMessage      `json:"configuration,omitempty"`
	Config          json.RawMessage      `json:"config,omitempty"`
	ConfigJSON      json.RawMessage      `json:"config_json,omitempty"`
}

// ListIntegrations handles GET /integrations.
func (h *IntegrationHandler) ListIntegrations(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusUnauthorized, "Missing organization context", "")
		return
	}

	integrations, err := h.svc.ListIntegrations(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list integrations", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": integrations})
}

// CreateIntegration handles POST /integrations.
func (h *IntegrationHandler) CreateIntegration(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	if orgID == "" || userID == "" {
		writeError(w, http.StatusUnauthorized, "Missing authentication context", "")
		return
	}

	var body integrationPayload
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	integration, configJSON, err := normalizeIntegrationPayload(body, true)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid integration", err.Error())
		return
	}

	result, err := h.svc.CreateIntegration(r.Context(), orgID, userID, integration, *configJSON)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create integration", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{"data": result})
}

// GetIntegration handles GET /integrations/{id}.
func (h *IntegrationHandler) GetIntegration(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusUnauthorized, "Missing organization context", "")
		return
	}

	integID := chi.URLParam(r, "id")
	if integID == "" {
		writeError(w, http.StatusBadRequest, "Missing integration ID", "")
		return
	}

	integration, err := h.svc.GetIntegration(r.Context(), orgID, integID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Integration not found", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": integration})
}

// UpdateIntegration handles PUT /integrations/{id}.
func (h *IntegrationHandler) UpdateIntegration(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusUnauthorized, "Missing organization context", "")
		return
	}

	integID := chi.URLParam(r, "id")
	if integID == "" {
		writeError(w, http.StatusBadRequest, "Missing integration ID", "")
		return
	}

	var body integrationPayload
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	integration, configJSON, err := normalizeIntegrationPayload(body, false)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid integration", err.Error())
		return
	}

	if err := h.svc.UpdateIntegration(r.Context(), orgID, integID, integration, configJSON); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update integration", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"message": "Integration updated"})
}

// DeleteIntegration handles DELETE /integrations/{id}.
func (h *IntegrationHandler) DeleteIntegration(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusUnauthorized, "Missing organization context", "")
		return
	}

	integID := chi.URLParam(r, "id")
	if integID == "" {
		writeError(w, http.StatusBadRequest, "Missing integration ID", "")
		return
	}

	if err := h.svc.DeleteIntegration(r.Context(), orgID, integID); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to delete integration", err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// TestConnection handles POST /integrations/{id}/test.
func (h *IntegrationHandler) TestConnection(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusUnauthorized, "Missing organization context", "")
		return
	}

	integID := chi.URLParam(r, "id")
	if integID == "" {
		writeError(w, http.StatusBadRequest, "Missing integration ID", "")
		return
	}

	status, err := h.svc.TestConnection(r.Context(), orgID, integID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Connection test failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  status,
		"message": "Connection test completed",
	})
}

// TriggerSync handles POST /integrations/{id}/sync.
func (h *IntegrationHandler) TriggerSync(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusUnauthorized, "Missing organization context", "")
		return
	}

	integID := chi.URLParam(r, "id")
	if integID == "" {
		writeError(w, http.StatusBadRequest, "Missing integration ID", "")
		return
	}

	var body struct {
		SyncType string `json:"sync_type"`
	}
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
			return
		}
	}

	if body.SyncType == "" {
		body.SyncType = "full"
	}

	result, err := h.svc.TriggerSync(r.Context(), orgID, integID, body.SyncType)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to trigger sync", err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]interface{}{"data": result})
}

// GetSyncLogs handles GET /integrations/{id}/logs.
func (h *IntegrationHandler) GetSyncLogs(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusUnauthorized, "Missing organization context", "")
		return
	}

	integID := chi.URLParam(r, "id")
	if integID == "" {
		writeError(w, http.StatusBadRequest, "Missing integration ID", "")
		return
	}

	pagination := parsePagination(r)

	logs, total, err := h.svc.GetSyncLogs(r.Context(), orgID, integID, pagination.Page, pagination.PageSize)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get sync logs", err.Error())
		return
	}

	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": logs,
		"pagination": models.PaginationResponse{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}

// GetSSOConfig handles GET /settings/sso.
func (h *IntegrationHandler) GetSSOConfig(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusUnauthorized, "Missing organization context", "")
		return
	}

	config, err := h.svc.GetSSOConfig(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get SSO configuration", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": config})
}

// UpdateSSOConfig handles PUT /settings/sso.
func (h *IntegrationHandler) UpdateSSOConfig(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusUnauthorized, "Missing organization context", "")
		return
	}

	var body service.UpdateSSOConfigurationInput
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if err := h.svc.UpdateSSOConfig(r.Context(), orgID, body); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update SSO configuration", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"message": "SSO configuration updated"})
}

// ListAPIKeys handles GET /settings/api-keys.
func (h *IntegrationHandler) ListAPIKeys(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusUnauthorized, "Missing organization context", "")
		return
	}

	keys, err := h.svc.ListAPIKeys(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list API keys", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": keys})
}

// CreateAPIKey handles POST /settings/api-keys.
func (h *IntegrationHandler) CreateAPIKey(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	if orgID == "" || userID == "" {
		writeError(w, http.StatusUnauthorized, "Missing authentication context", "")
		return
	}

	var body struct {
		Name        string   `json:"name"`
		Permissions []string `json:"permissions"`
		RateLimit   int      `json:"rate_limit"`
		ExpiresAt   *string  `json:"expires_at,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if body.Name == "" {
		writeError(w, http.StatusBadRequest, "Name is required", "")
		return
	}

	var expiresAt *time.Time
	if body.ExpiresAt != nil {
		parsed, err := time.Parse(time.RFC3339, *body.ExpiresAt)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Invalid expiry", "expires_at must use RFC3339")
			return
		}
		if !parsed.After(time.Now().UTC()) {
			writeError(w, http.StatusBadRequest, "Invalid expiry", "expires_at must be in the future")
			return
		}
		expiresAt = &parsed
	}
	if err := validateAPIKeyPermissions(body.Permissions); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid permissions", err.Error())
		return
	}

	keyRecord, rawKey, err := h.svc.CreateAPIKey(r.Context(), orgID, userID, body.Name, body.Permissions, body.RateLimit, expiresAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create API key", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"data": keyRecord,
		"key":  rawKey,
		"note": "Store this key securely. It will not be shown again.",
	})
}

// RevokeAPIKey handles DELETE /settings/api-keys/{id}.
func (h *IntegrationHandler) RevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusUnauthorized, "Missing organization context", "")
		return
	}

	keyID := chi.URLParam(r, "id")
	if keyID == "" {
		writeError(w, http.StatusBadRequest, "Missing API key ID", "")
		return
	}

	if err := h.svc.RevokeAPIKey(r.Context(), orgID, keyID); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to revoke API key", err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func normalizeIntegrationPayload(payload integrationPayload, requireConfiguration bool) (service.Integration, *string, error) {
	var integration service.Integration
	if payload.Integration != nil {
		integration = *payload.Integration
	}
	if payload.IntegrationType != "" {
		integration.IntegrationType = payload.IntegrationType
	} else if payload.Type != "" {
		integration.IntegrationType = payload.Type
	}
	if payload.Name != "" {
		integration.Name = payload.Name
	}
	if payload.Description != nil {
		integration.Description = payload.Description
	}
	if payload.SyncFrequency != 0 {
		integration.SyncFreqMinutes = payload.SyncFrequency
	}
	if payload.Capabilities != nil {
		integration.Capabilities = payload.Capabilities
	}

	integration.IntegrationType = normalizeIntegrationType(integration.IntegrationType)
	integration.Name = strings.TrimSpace(integration.Name)
	if integration.IntegrationType != "" && !validIntegrationType(integration.IntegrationType) {
		return service.Integration{}, nil, errors.New("unsupported integration type")
	}
	if requireConfiguration && integration.IntegrationType == "" {
		return service.Integration{}, nil, errors.New("integration type is required")
	}
	if requireConfiguration && integration.Name == "" {
		return service.Integration{}, nil, errors.New("name is required")
	}
	if len(integration.Name) > 200 {
		return service.Integration{}, nil, errors.New("name must not exceed 200 characters")
	}
	if integration.SyncFreqMinutes < 0 || integration.SyncFreqMinutes > 525_600 {
		return service.Integration{}, nil, errors.New("sync frequency must be between 0 and 525600 minutes")
	}
	if len(integration.Capabilities) > 100 {
		return service.Integration{}, nil, errors.New("no more than 100 capabilities may be configured")
	}
	for _, capability := range integration.Capabilities {
		if !validScopeSegment(capability) {
			return service.Integration{}, nil, errors.New("capabilities must use lowercase letters, digits, underscores, dots, or hyphens")
		}
	}

	rawConfig := payload.Configuration
	if len(rawConfig) == 0 {
		rawConfig = payload.Config
	}
	if len(rawConfig) == 0 {
		rawConfig = payload.ConfigJSON
	}
	configJSON, err := normalizeConfigurationJSON(rawConfig)
	if err != nil {
		return service.Integration{}, nil, err
	}
	if requireConfiguration && configJSON == nil {
		return service.Integration{}, nil, errors.New("configuration is required")
	}
	return integration, configJSON, nil
}

func normalizeConfigurationJSON(raw json.RawMessage) (*string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	if len(raw) > 64*1024 {
		return nil, errors.New("configuration must not exceed 64 KiB")
	}

	if raw[0] == '"' {
		var encoded string
		if err := json.Unmarshal(raw, &encoded); err != nil {
			return nil, errors.New("config_json must contain valid JSON")
		}
		raw = json.RawMessage(encoded)
	}
	var object map[string]any
	if !json.Valid(raw) || json.Unmarshal(raw, &object) != nil || object == nil {
		return nil, errors.New("configuration must be a JSON object")
	}
	canonical, err := json.Marshal(object)
	if err != nil {
		return nil, errors.New("configuration could not be encoded")
	}
	value := string(canonical)
	return &value, nil
}

func normalizeIntegrationType(value string) string {
	aliases := map[string]string{
		"saml":       "sso_saml",
		"oidc":       "sso_oidc",
		"aws":        "cloud_aws",
		"azure":      "cloud_azure",
		"gcp":        "cloud_gcp",
		"splunk":     "siem_splunk",
		"elastic":    "siem_elastic",
		"servicenow": "itsm_servicenow",
		"jira":       "itsm_jira",
		"webhook":    "webhook_outbound",
	}
	normalized := strings.ToLower(strings.TrimSpace(value))
	if alias, ok := aliases[normalized]; ok {
		return alias
	}
	return normalized
}

func validIntegrationType(value string) bool {
	valid := map[string]struct{}{
		"sso_saml": {}, "sso_oidc": {},
		"cloud_aws": {}, "cloud_azure": {}, "cloud_gcp": {},
		"siem_splunk": {}, "siem_elastic": {}, "siem_sentinel": {},
		"itsm_servicenow": {}, "itsm_jira": {}, "itsm_freshservice": {},
		"email_smtp": {}, "email_sendgrid": {}, "slack": {}, "teams": {},
		"webhook_inbound": {}, "webhook_outbound": {}, "custom_api": {},
	}
	_, ok := valid[value]
	return ok
}

func validateAPIKeyPermissions(permissions []string) error {
	if len(permissions) == 0 {
		return errors.New("at least one permission is required")
	}
	if len(permissions) > 100 {
		return errors.New("no more than 100 permissions may be granted")
	}
	validActions := map[string]struct{}{
		"create": {}, "read": {}, "update": {}, "delete": {},
		"approve": {}, "assign": {}, "export": {}, "configure": {},
	}
	for _, permission := range permissions {
		parts := strings.Split(permission, ":")
		if len(parts) != 2 {
			return errors.New("permissions must use action:resource format")
		}
		if _, ok := validActions[parts[0]]; !ok || !validScopeSegment(parts[1]) {
			return errors.New("permission contains an unsupported action or resource")
		}
	}
	return nil
}

func validScopeSegment(value string) bool {
	if value == "" || len(value) > 100 {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') ||
			(character >= '0' && character <= '9') ||
			character == '_' || character == '-' || character == '.' {
			continue
		}
		return false
	}
	return true
}

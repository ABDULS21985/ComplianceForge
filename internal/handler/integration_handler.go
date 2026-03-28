package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/complianceforge/platform/internal/middleware"
	"github.com/complianceforge/platform/internal/models"
)

// IntegrationSvc defines the methods required by IntegrationHandler.
type IntegrationSvc interface {
	ListIntegrations(ctx context.Context, orgID string) ([]interface{}, error)
	CreateIntegration(ctx context.Context, orgID, userID string, integ interface{}, configJSON string) (interface{}, error)
	GetIntegration(ctx context.Context, orgID, integID string) (interface{}, error)
	UpdateIntegration(ctx context.Context, orgID, integID string, integ interface{}, configJSON *string) error
	DeleteIntegration(ctx context.Context, orgID, integID string) error
	TestConnection(ctx context.Context, orgID, integID string) (string, error)
	TriggerSync(ctx context.Context, orgID, integID, syncType string) (interface{}, error)
	GetSyncLogs(ctx context.Context, orgID, integID string, page, pageSize int) ([]interface{}, int, error)
	GetSSOConfig(ctx context.Context, orgID string) (interface{}, error)
	UpdateSSOConfig(ctx context.Context, orgID string, config interface{}) error
	ListAPIKeys(ctx context.Context, orgID string) ([]interface{}, error)
	CreateAPIKey(ctx context.Context, orgID, userID, name string, permissions []string, rateLimit int, expiresAt interface{}) (interface{}, string, error)
	RevokeAPIKey(ctx context.Context, orgID, keyID string) error
}

// IntegrationHandler handles integration management endpoints.
type IntegrationHandler struct {
	svc IntegrationSvc
}

// NewIntegrationHandler creates a new IntegrationHandler with the given service.
func NewIntegrationHandler(svc IntegrationSvc) *IntegrationHandler {
	return &IntegrationHandler{svc: svc}
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

	var body struct {
		Integration map[string]interface{} `json:"integration"`
		ConfigJSON  string                 `json:"config_json"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	result, err := h.svc.CreateIntegration(r.Context(), orgID, userID, body.Integration, body.ConfigJSON)
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

	var body struct {
		Integration map[string]interface{} `json:"integration"`
		ConfigJSON  *string                `json:"config_json,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if err := h.svc.UpdateIntegration(r.Context(), orgID, integID, body.Integration, body.ConfigJSON); err != nil {
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
	json.NewDecoder(r.Body).Decode(&body)

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

	var body map[string]interface{}
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

	var expiresAt interface{}
	if body.ExpiresAt != nil {
		expiresAt = *body.ExpiresAt
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

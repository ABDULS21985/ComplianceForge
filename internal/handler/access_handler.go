package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/complianceforge/platform/internal/middleware"
	"github.com/complianceforge/platform/internal/models"
)

// ABACService defines the methods required by AccessHandler.
type ABACService interface {
	Evaluate(ctx context.Context, req interface{}) (interface{}, error)
	GetUserPermissions(ctx context.Context, orgID, userID string) (map[string][]string, error)
	GetFieldPermissions(ctx context.Context, orgID, userID, resourceType string) ([]interface{}, error)
	ListPolicies(ctx context.Context, orgID string) ([]interface{}, error)
	CreatePolicy(ctx context.Context, orgID, userID string, policy interface{}) (interface{}, error)
	UpdatePolicy(ctx context.Context, orgID string, policy interface{}) error
	DeletePolicy(ctx context.Context, orgID, policyID string) error
	AssignPolicy(ctx context.Context, orgID, policyID, assigneeType, assigneeID, createdBy string) error
	RemoveAssignment(ctx context.Context, orgID, assignmentID string) error
	GetAuditLog(ctx context.Context, orgID string, page, pageSize int) ([]interface{}, int, error)
}

// AccessHandler handles ABAC access control endpoints.
type AccessHandler struct {
	svc ABACService
}

// NewAccessHandler creates a new AccessHandler with the given service.
func NewAccessHandler(svc ABACService) *AccessHandler {
	return &AccessHandler{svc: svc}
}

// ListPolicies handles GET /access/policies.
func (h *AccessHandler) ListPolicies(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusUnauthorized, "Missing organization context", "")
		return
	}

	policies, err := h.svc.ListPolicies(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list access policies", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": policies})
}

// CreatePolicy handles POST /access/policies.
func (h *AccessHandler) CreatePolicy(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	if orgID == "" || userID == "" {
		writeError(w, http.StatusUnauthorized, "Missing authentication context", "")
		return
	}

	var body map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	policy, err := h.svc.CreatePolicy(r.Context(), orgID, userID, body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create access policy", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{"data": policy})
}

// UpdatePolicy handles PUT /access/policies/{id}.
func (h *AccessHandler) UpdatePolicy(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusUnauthorized, "Missing organization context", "")
		return
	}

	policyID := chi.URLParam(r, "id")
	if policyID == "" {
		writeError(w, http.StatusBadRequest, "Missing policy ID", "")
		return
	}

	var body map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	// Inject the policy ID into the body for the service layer.
	body["id"] = policyID

	if err := h.svc.UpdatePolicy(r.Context(), orgID, body); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update access policy", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"message": "Policy updated"})
}

// DeletePolicy handles DELETE /access/policies/{id}.
func (h *AccessHandler) DeletePolicy(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusUnauthorized, "Missing organization context", "")
		return
	}

	policyID := chi.URLParam(r, "id")
	if policyID == "" {
		writeError(w, http.StatusBadRequest, "Missing policy ID", "")
		return
	}

	if err := h.svc.DeletePolicy(r.Context(), orgID, policyID); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to delete access policy", err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// AssignPolicy handles POST /access/policies/{id}/assignments.
func (h *AccessHandler) AssignPolicy(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	if orgID == "" || userID == "" {
		writeError(w, http.StatusUnauthorized, "Missing authentication context", "")
		return
	}

	policyID := chi.URLParam(r, "id")
	if policyID == "" {
		writeError(w, http.StatusBadRequest, "Missing policy ID", "")
		return
	}

	var body struct {
		AssigneeType string `json:"assignee_type"`
		AssigneeID   string `json:"assignee_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if body.AssigneeType == "" || body.AssigneeID == "" {
		writeError(w, http.StatusBadRequest, "assignee_type and assignee_id are required", "")
		return
	}

	if err := h.svc.AssignPolicy(r.Context(), orgID, policyID, body.AssigneeType, body.AssigneeID, userID); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to assign policy", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{"message": "Policy assigned"})
}

// RemoveAssignment handles DELETE /access/policies/{id}/assignments/{assignmentId}.
func (h *AccessHandler) RemoveAssignment(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusUnauthorized, "Missing organization context", "")
		return
	}

	assignmentID := chi.URLParam(r, "assignmentId")
	if assignmentID == "" {
		writeError(w, http.StatusBadRequest, "Missing assignment ID", "")
		return
	}

	if err := h.svc.RemoveAssignment(r.Context(), orgID, assignmentID); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to remove assignment", err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// TestEvaluate handles POST /access/evaluate.
func (h *AccessHandler) TestEvaluate(w http.ResponseWriter, r *http.Request) {
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

	// Inject org context for the evaluation engine.
	body["organization_id"] = orgID

	result, err := h.svc.Evaluate(r.Context(), body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to evaluate access request", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": result})
}

// GetAuditLog handles GET /access/audit-log.
func (h *AccessHandler) GetAuditLog(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusUnauthorized, "Missing organization context", "")
		return
	}

	pagination := parsePagination(r)

	logs, total, err := h.svc.GetAuditLog(r.Context(), orgID, pagination.Page, pagination.PageSize)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get access audit log", err.Error())
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

// GetMyPermissions handles GET /access/my-permissions.
func (h *AccessHandler) GetMyPermissions(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	if orgID == "" || userID == "" {
		writeError(w, http.StatusUnauthorized, "Missing authentication context", "")
		return
	}

	permissions, err := h.svc.GetUserPermissions(r.Context(), orgID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get permissions", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": permissions})
}

// GetFieldPermissions handles GET /access/field-permissions.
func (h *AccessHandler) GetFieldPermissions(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	if orgID == "" || userID == "" {
		writeError(w, http.StatusUnauthorized, "Missing authentication context", "")
		return
	}

	resourceType := r.URL.Query().Get("resource_type")
	if resourceType == "" {
		writeError(w, http.StatusBadRequest, "resource_type query parameter is required", "")
		return
	}

	fields, err := h.svc.GetFieldPermissions(r.Context(), orgID, userID, resourceType)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get field permissions", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": fields})
}

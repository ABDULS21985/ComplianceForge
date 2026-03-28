package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/complianceforge/platform/internal/middleware"
	"github.com/complianceforge/platform/internal/models"
)

// WorkflowService defines the methods required by WorkflowHandler.
type WorkflowService interface {
	StartWorkflow(ctx context.Context, orgID, workflowType, entityType, entityID, entityRef, startedBy string) (interface{}, error)
	ProcessStep(ctx context.Context, orgID, executionID, action, actorID, comments, reason string) error
	DelegateStep(ctx context.Context, orgID, executionID, delegatorID, delegateID string) error
	CancelWorkflow(ctx context.Context, orgID, instanceID, cancelledBy, reason string) error
	GetPendingApprovals(ctx context.Context, orgID, userID string, page, pageSize int) ([]interface{}, int, error)
	GetWorkflowHistory(ctx context.Context, orgID, entityType, entityID string) ([]interface{}, error)
	ListDefinitions(ctx context.Context, orgID string) ([]interface{}, error)
	CreateDefinition(ctx context.Context, orgID string, def interface{}) (interface{}, error)
	UpdateDefinition(ctx context.Context, orgID, defID string, def interface{}) error
	ActivateDefinition(ctx context.Context, orgID, defID string) error
	GetInstanceDetail(ctx context.Context, orgID, instanceID string) (interface{}, error)
	ListInstances(ctx context.Context, orgID string, pagination models.PaginationRequest, entityType, status string) ([]interface{}, int, error)
	ListDelegations(ctx context.Context, orgID string) ([]interface{}, error)
	CreateDelegation(ctx context.Context, orgID string, delegation interface{}) (interface{}, error)
}

// WorkflowHandler handles workflow management endpoints.
type WorkflowHandler struct {
	svc WorkflowService
}

// NewWorkflowHandler creates a new WorkflowHandler with the given service.
func NewWorkflowHandler(svc WorkflowService) *WorkflowHandler {
	return &WorkflowHandler{svc: svc}
}

// ListDefinitions handles GET /workflows/definitions.
func (h *WorkflowHandler) ListDefinitions(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusUnauthorized, "Missing organization context", "")
		return
	}

	defs, err := h.svc.ListDefinitions(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list workflow definitions", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": defs})
}

// CreateDefinition handles POST /workflows/definitions.
func (h *WorkflowHandler) CreateDefinition(w http.ResponseWriter, r *http.Request) {
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

	def, err := h.svc.CreateDefinition(r.Context(), orgID, body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create workflow definition", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{"data": def})
}

// UpdateDefinition handles PUT /workflows/definitions/{id}.
func (h *WorkflowHandler) UpdateDefinition(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusUnauthorized, "Missing organization context", "")
		return
	}

	defID := chi.URLParam(r, "id")
	if defID == "" {
		writeError(w, http.StatusBadRequest, "Missing definition ID", "")
		return
	}

	var body map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if err := h.svc.UpdateDefinition(r.Context(), orgID, defID, body); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update workflow definition", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"message": "Definition updated"})
}

// ActivateDefinition handles POST /workflows/definitions/{id}/activate.
func (h *WorkflowHandler) ActivateDefinition(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusUnauthorized, "Missing organization context", "")
		return
	}

	defID := chi.URLParam(r, "id")
	if defID == "" {
		writeError(w, http.StatusBadRequest, "Missing definition ID", "")
		return
	}

	if err := h.svc.ActivateDefinition(r.Context(), orgID, defID); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to activate workflow definition", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"message": "Definition activated"})
}

// ListInstances handles GET /workflows/instances.
func (h *WorkflowHandler) ListInstances(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusUnauthorized, "Missing organization context", "")
		return
	}

	pagination := parsePagination(r)
	entityType := r.URL.Query().Get("entity_type")
	status := r.URL.Query().Get("status")

	instances, total, err := h.svc.ListInstances(r.Context(), orgID, pagination, entityType, status)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list workflow instances", err.Error())
		return
	}

	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": instances,
		"pagination": models.PaginationResponse{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}

// GetInstance handles GET /workflows/instances/{id}.
func (h *WorkflowHandler) GetInstance(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusUnauthorized, "Missing organization context", "")
		return
	}

	instanceID := chi.URLParam(r, "id")
	if instanceID == "" {
		writeError(w, http.StatusBadRequest, "Missing instance ID", "")
		return
	}

	instance, err := h.svc.GetInstanceDetail(r.Context(), orgID, instanceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Workflow instance not found", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": instance})
}

// StartWorkflow handles POST /workflows/start.
func (h *WorkflowHandler) StartWorkflow(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	if orgID == "" || userID == "" {
		writeError(w, http.StatusUnauthorized, "Missing authentication context", "")
		return
	}

	var body struct {
		WorkflowType string `json:"workflow_type"`
		EntityType   string `json:"entity_type"`
		EntityID     string `json:"entity_id"`
		EntityRef    string `json:"entity_ref"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if body.WorkflowType == "" || body.EntityType == "" || body.EntityID == "" {
		writeError(w, http.StatusBadRequest, "workflow_type, entity_type, and entity_id are required", "")
		return
	}

	result, err := h.svc.StartWorkflow(r.Context(), orgID, body.WorkflowType, body.EntityType, body.EntityID, body.EntityRef, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to start workflow", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{"data": result})
}

// CancelWorkflow handles POST /workflows/instances/{id}/cancel.
func (h *WorkflowHandler) CancelWorkflow(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	if orgID == "" || userID == "" {
		writeError(w, http.StatusUnauthorized, "Missing authentication context", "")
		return
	}

	instanceID := chi.URLParam(r, "id")
	if instanceID == "" {
		writeError(w, http.StatusBadRequest, "Missing instance ID", "")
		return
	}

	var body struct {
		Reason string `json:"reason"`
	}
	json.NewDecoder(r.Body).Decode(&body)

	if err := h.svc.CancelWorkflow(r.Context(), orgID, instanceID, userID, body.Reason); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to cancel workflow", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"message": "Workflow cancelled"})
}

// GetMyApprovals handles GET /workflows/my-approvals.
func (h *WorkflowHandler) GetMyApprovals(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	if orgID == "" || userID == "" {
		writeError(w, http.StatusUnauthorized, "Missing authentication context", "")
		return
	}

	pagination := parsePagination(r)

	approvals, total, err := h.svc.GetPendingApprovals(r.Context(), orgID, userID, pagination.Page, pagination.PageSize)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get pending approvals", err.Error())
		return
	}

	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": approvals,
		"pagination": models.PaginationResponse{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}

// ApproveStep handles POST /workflows/executions/{id}/approve.
func (h *WorkflowHandler) ApproveStep(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	if orgID == "" || userID == "" {
		writeError(w, http.StatusUnauthorized, "Missing authentication context", "")
		return
	}

	executionID := chi.URLParam(r, "id")
	if executionID == "" {
		writeError(w, http.StatusBadRequest, "Missing execution ID", "")
		return
	}

	var body struct {
		Comments string `json:"comments"`
	}
	json.NewDecoder(r.Body).Decode(&body)

	if err := h.svc.ProcessStep(r.Context(), orgID, executionID, "approve", userID, body.Comments, ""); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to approve step", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"message": "Step approved"})
}

// RejectStep handles POST /workflows/executions/{id}/reject.
func (h *WorkflowHandler) RejectStep(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	if orgID == "" || userID == "" {
		writeError(w, http.StatusUnauthorized, "Missing authentication context", "")
		return
	}

	executionID := chi.URLParam(r, "id")
	if executionID == "" {
		writeError(w, http.StatusBadRequest, "Missing execution ID", "")
		return
	}

	var body struct {
		Comments string `json:"comments"`
		Reason   string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if body.Reason == "" {
		writeError(w, http.StatusBadRequest, "Reason is required for rejection", "")
		return
	}

	if err := h.svc.ProcessStep(r.Context(), orgID, executionID, "reject", userID, body.Comments, body.Reason); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to reject step", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"message": "Step rejected"})
}

// DelegateStep handles POST /workflows/executions/{id}/delegate.
func (h *WorkflowHandler) DelegateStep(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	if orgID == "" || userID == "" {
		writeError(w, http.StatusUnauthorized, "Missing authentication context", "")
		return
	}

	executionID := chi.URLParam(r, "id")
	if executionID == "" {
		writeError(w, http.StatusBadRequest, "Missing execution ID", "")
		return
	}

	var body struct {
		DelegateID string `json:"delegate_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if body.DelegateID == "" {
		writeError(w, http.StatusBadRequest, "delegate_id is required", "")
		return
	}

	if err := h.svc.DelegateStep(r.Context(), orgID, executionID, userID, body.DelegateID); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to delegate step", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"message": "Step delegated"})
}

// RequestInfo handles POST /workflows/executions/{id}/request-info.
func (h *WorkflowHandler) RequestInfo(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	if orgID == "" || userID == "" {
		writeError(w, http.StatusUnauthorized, "Missing authentication context", "")
		return
	}

	executionID := chi.URLParam(r, "id")
	if executionID == "" {
		writeError(w, http.StatusBadRequest, "Missing execution ID", "")
		return
	}

	var body struct {
		Comments string `json:"comments"`
		Reason   string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if err := h.svc.ProcessStep(r.Context(), orgID, executionID, "request_info", userID, body.Comments, body.Reason); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to request info", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"message": "Information requested"})
}

// ListDelegations handles GET /workflows/delegations.
func (h *WorkflowHandler) ListDelegations(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusUnauthorized, "Missing organization context", "")
		return
	}

	delegations, err := h.svc.ListDelegations(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list delegations", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": delegations})
}

// CreateDelegation handles POST /workflows/delegations.
func (h *WorkflowHandler) CreateDelegation(w http.ResponseWriter, r *http.Request) {
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

	delegation, err := h.svc.CreateDelegation(r.Context(), orgID, body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create delegation", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{"data": delegation})
}

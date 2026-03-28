package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/complianceforge/platform/internal/middleware"
	"github.com/complianceforge/platform/internal/models"
)

// ---------- service interface ----------

// DSRService defines the methods required by DSRHandler.
type DSRService interface {
	ListRequests(ctx context.Context, orgID string, pagination models.PaginationRequest, filters DSRFilters) ([]DSRRequest, int, error)
	GetRequest(ctx context.Context, orgID, requestID string) (*DSRRequestDetail, error)
	CreateRequest(ctx context.Context, orgID, userID string, req *DSRRequest) error
	UpdateRequest(ctx context.Context, orgID string, req *DSRRequest) error
	VerifyIdentity(ctx context.Context, orgID, userID, requestID string, verification *IdentityVerification) error
	AssignRequest(ctx context.Context, orgID, requestID, assigneeID string) error
	ExtendDeadline(ctx context.Context, orgID, userID, requestID string, extension *DeadlineExtension) error
	CompleteRequest(ctx context.Context, orgID, userID, requestID string, completion *DSRCompletion) error
	RejectRequest(ctx context.Context, orgID, userID, requestID string, rejection *DSRRejection) error
	UpdateTask(ctx context.Context, orgID, requestID, taskID string, update *DSRTaskUpdate) error
	GetDashboard(ctx context.Context, orgID string) (*DSRDashboard, error)
	GetOverdue(ctx context.Context, orgID string) ([]DSRRequest, error)
	ListTemplates(ctx context.Context, orgID string) ([]DSRTemplate, error)
}

// ---------- request / response types ----------

// DSRFilters holds filter parameters for listing DSR requests.
type DSRFilters struct {
	Status      string `json:"status"`
	RequestType string `json:"request_type"`
	AssigneeID  string `json:"assignee_id"`
	Search      string `json:"search"`
}

// DSRRequest represents a data subject request.
type DSRRequest struct {
	ID              string `json:"id"`
	OrganizationID  string `json:"organization_id"`
	RequestType     string `json:"request_type" validate:"required"` // access, erasure, rectification, portability, restriction, objection
	Status          string `json:"status"`                           // pending, verified, in_progress, completed, rejected
	DataSubjectName string `json:"data_subject_name" validate:"required"`
	DataSubjectEmail string `json:"data_subject_email" validate:"required"`
	Description     string `json:"description"`
	LegalBasis      string `json:"legal_basis,omitempty"`
	AssigneeID      string `json:"assignee_id,omitempty"`
	DueDate         string `json:"due_date,omitempty"`
	CreatedBy       string `json:"created_by"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
	CompletedAt     string `json:"completed_at,omitempty"`
}

// DSRRequestDetail extends DSRRequest with tasks and audit trail.
type DSRRequestDetail struct {
	DSRRequest
	Tasks      []DSRTask      `json:"tasks"`
	AuditTrail []DSRAuditEntry `json:"audit_trail"`
}

// DSRTask represents an individual task within a DSR request workflow.
type DSRTask struct {
	ID        string `json:"id"`
	RequestID string `json:"request_id"`
	Title     string `json:"title"`
	Status    string `json:"status"` // pending, in_progress, completed, skipped
	AssigneeID string `json:"assignee_id,omitempty"`
	Notes     string `json:"notes,omitempty"`
	DueDate   string `json:"due_date,omitempty"`
	CompletedAt string `json:"completed_at,omitempty"`
}

// DSRTaskUpdate is the payload for PUT /dsr/{id}/tasks/{taskId}.
type DSRTaskUpdate struct {
	Status string `json:"status" validate:"required"`
	Notes  string `json:"notes,omitempty"`
}

// DSRAuditEntry is an immutable log entry for DSR request changes.
type DSRAuditEntry struct {
	ID        string `json:"id"`
	RequestID string `json:"request_id"`
	Action    string `json:"action"`
	UserID    string `json:"user_id"`
	Details   string `json:"details,omitempty"`
	CreatedAt string `json:"created_at"`
}

// IdentityVerification is the payload for POST /dsr/{id}/verify-identity.
type IdentityVerification struct {
	Method   string `json:"method" validate:"required"` // document, email, phone, in_person
	Verified bool   `json:"verified"`
	Notes    string `json:"notes,omitempty"`
}

// DeadlineExtension is the payload for POST /dsr/{id}/extend.
type DeadlineExtension struct {
	NewDueDate string `json:"new_due_date" validate:"required"`
	Reason     string `json:"reason" validate:"required"`
}

// DSRCompletion is the payload for POST /dsr/{id}/complete.
type DSRCompletion struct {
	ResponseSummary string `json:"response_summary"`
	Notes           string `json:"notes,omitempty"`
}

// DSRRejection is the payload for POST /dsr/{id}/reject.
type DSRRejection struct {
	LegalBasis string `json:"legal_basis" validate:"required"`
	Reason     string `json:"reason" validate:"required"`
}

// DSRDashboard provides aggregate DSR metrics.
type DSRDashboard struct {
	TotalRequests     int            `json:"total_requests"`
	OpenRequests      int            `json:"open_requests"`
	CompletedRequests int            `json:"completed_requests"`
	OverdueRequests   int            `json:"overdue_requests"`
	AverageCloseDays  float64        `json:"average_close_days"`
	ByType            map[string]int `json:"by_type"`
	ByStatus          map[string]int `json:"by_status"`
}

// DSRTemplate is a reusable response template for DSR communications.
type DSRTemplate struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	RequestType string `json:"request_type"`
	Subject     string `json:"subject"`
	Body        string `json:"body"`
}

// ---------- handler ----------

// DSRHandler handles data subject request endpoints.
type DSRHandler struct {
	svc DSRService
}

// NewDSRHandler creates a new DSRHandler with the given service.
func NewDSRHandler(svc DSRService) *DSRHandler {
	return &DSRHandler{svc: svc}
}

// ListRequests handles GET /dsr.
func (h *DSRHandler) ListRequests(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	pagination := parsePagination(r)

	filters := DSRFilters{
		Status:      r.URL.Query().Get("status"),
		RequestType: r.URL.Query().Get("request_type"),
		AssigneeID:  r.URL.Query().Get("assignee_id"),
		Search:      r.URL.Query().Get("search"),
	}

	requests, total, err := h.svc.ListRequests(r.Context(), orgID, pagination, filters)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list DSR requests", err.Error())
		return
	}

	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": requests,
		"pagination": models.PaginationResponse{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}

// GetRequest handles GET /dsr/{id}.
func (h *DSRHandler) GetRequest(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	requestID := chi.URLParam(r, "id")
	if requestID == "" {
		writeError(w, http.StatusBadRequest, "Missing request ID", "")
		return
	}

	detail, err := h.svc.GetRequest(r.Context(), orgID, requestID)
	if err != nil {
		writeError(w, http.StatusNotFound, "DSR request not found", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, detail)
}

// CreateRequest handles POST /dsr.
func (h *DSRHandler) CreateRequest(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	var req DSRRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if req.RequestType == "" || req.DataSubjectName == "" || req.DataSubjectEmail == "" {
		writeError(w, http.StatusBadRequest, "request_type, data_subject_name, and data_subject_email are required", "")
		return
	}

	if err := h.svc.CreateRequest(r.Context(), orgID, userID, &req); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create DSR request", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, req)
}

// UpdateRequest handles PUT /dsr/{id}.
func (h *DSRHandler) UpdateRequest(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	requestID := chi.URLParam(r, "id")
	if requestID == "" {
		writeError(w, http.StatusBadRequest, "Missing request ID", "")
		return
	}

	var req DSRRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	req.ID = requestID
	req.OrganizationID = orgID

	if err := h.svc.UpdateRequest(r.Context(), orgID, &req); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update DSR request", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, req)
}

// VerifyIdentity handles POST /dsr/{id}/verify-identity.
func (h *DSRHandler) VerifyIdentity(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	requestID := chi.URLParam(r, "id")
	if requestID == "" {
		writeError(w, http.StatusBadRequest, "Missing request ID", "")
		return
	}

	var verification IdentityVerification
	if err := json.NewDecoder(r.Body).Decode(&verification); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if verification.Method == "" {
		writeError(w, http.StatusBadRequest, "method is required", "")
		return
	}

	if err := h.svc.VerifyIdentity(r.Context(), orgID, userID, requestID, &verification); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to verify identity", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Identity verification recorded"})
}

// AssignRequest handles POST /dsr/{id}/assign.
func (h *DSRHandler) AssignRequest(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	requestID := chi.URLParam(r, "id")
	if requestID == "" {
		writeError(w, http.StatusBadRequest, "Missing request ID", "")
		return
	}

	var body struct {
		AssigneeID string `json:"assignee_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if body.AssigneeID == "" {
		writeError(w, http.StatusBadRequest, "assignee_id is required", "")
		return
	}

	if err := h.svc.AssignRequest(r.Context(), orgID, requestID, body.AssigneeID); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to assign DSR request", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Request assigned"})
}

// ExtendDeadline handles POST /dsr/{id}/extend.
func (h *DSRHandler) ExtendDeadline(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	requestID := chi.URLParam(r, "id")
	if requestID == "" {
		writeError(w, http.StatusBadRequest, "Missing request ID", "")
		return
	}

	var extension DeadlineExtension
	if err := json.NewDecoder(r.Body).Decode(&extension); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if extension.NewDueDate == "" || extension.Reason == "" {
		writeError(w, http.StatusBadRequest, "new_due_date and reason are required", "")
		return
	}

	if err := h.svc.ExtendDeadline(r.Context(), orgID, userID, requestID, &extension); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to extend deadline", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Deadline extended"})
}

// CompleteRequest handles POST /dsr/{id}/complete.
func (h *DSRHandler) CompleteRequest(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	requestID := chi.URLParam(r, "id")
	if requestID == "" {
		writeError(w, http.StatusBadRequest, "Missing request ID", "")
		return
	}

	var completion DSRCompletion
	if err := json.NewDecoder(r.Body).Decode(&completion); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if err := h.svc.CompleteRequest(r.Context(), orgID, userID, requestID, &completion); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to complete DSR request", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Request completed"})
}

// RejectRequest handles POST /dsr/{id}/reject.
func (h *DSRHandler) RejectRequest(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	requestID := chi.URLParam(r, "id")
	if requestID == "" {
		writeError(w, http.StatusBadRequest, "Missing request ID", "")
		return
	}

	var rejection DSRRejection
	if err := json.NewDecoder(r.Body).Decode(&rejection); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if rejection.LegalBasis == "" || rejection.Reason == "" {
		writeError(w, http.StatusBadRequest, "legal_basis and reason are required", "")
		return
	}

	if err := h.svc.RejectRequest(r.Context(), orgID, userID, requestID, &rejection); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to reject DSR request", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Request rejected"})
}

// UpdateTask handles PUT /dsr/{id}/tasks/{taskId}.
func (h *DSRHandler) UpdateTask(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	requestID := chi.URLParam(r, "id")
	taskID := chi.URLParam(r, "taskId")
	if requestID == "" || taskID == "" {
		writeError(w, http.StatusBadRequest, "Missing request ID or task ID", "")
		return
	}

	var update DSRTaskUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if update.Status == "" {
		writeError(w, http.StatusBadRequest, "status is required", "")
		return
	}

	if err := h.svc.UpdateTask(r.Context(), orgID, requestID, taskID, &update); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update task", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Task updated"})
}

// GetDashboard handles GET /dsr/dashboard.
func (h *DSRHandler) GetDashboard(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())

	dashboard, err := h.svc.GetDashboard(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get DSR dashboard", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": dashboard})
}

// GetOverdue handles GET /dsr/overdue.
func (h *DSRHandler) GetOverdue(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())

	requests, err := h.svc.GetOverdue(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get overdue DSR requests", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": requests})
}

// ListTemplates handles GET /dsr/templates.
func (h *DSRHandler) ListTemplates(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())

	templates, err := h.svc.ListTemplates(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list DSR templates", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": templates})
}

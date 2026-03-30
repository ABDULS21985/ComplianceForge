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

// ExceptionService defines the methods required by ExceptionHandler.
type ExceptionService interface {
	ListExceptions(ctx context.Context, orgID string, pagination models.PaginationRequest, filters ExceptionFilters) ([]Exception, int, error)
	CreateException(ctx context.Context, orgID, userID string, exception *Exception) error
	GetException(ctx context.Context, orgID, exceptionID string) (*ExceptionDetail, error)
	UpdateException(ctx context.Context, orgID string, exception *Exception) error
	SubmitException(ctx context.Context, orgID, userID, exceptionID string) error
	ApproveException(ctx context.Context, orgID, userID, exceptionID string, req *ExceptionApprovalRequest) error
	RejectException(ctx context.Context, orgID, userID, exceptionID string, req *ExceptionRejectionRequest) error
	RevokeException(ctx context.Context, orgID, userID, exceptionID string, req *ExceptionRevokeRequest) error
	RenewException(ctx context.Context, orgID, userID, exceptionID string, req *ExceptionRenewRequest) error
	ReviewException(ctx context.Context, orgID, userID, exceptionID string, req *ExceptionReviewRequest) error
	GetDashboard(ctx context.Context, orgID string) (*ExceptionDashboard, error)
	GetExpiring(ctx context.Context, orgID string, daysAhead int) ([]Exception, error)
	GetImpactAnalysis(ctx context.Context, orgID, exceptionID string) (*ExceptionImpactAnalysis, error)
}

// ---------- request / response types ----------

// ExceptionFilters holds filter parameters for listing exceptions.
type ExceptionFilters struct {
	Status     string `json:"status"`
	Type       string `json:"type"`
	RiskLevel  string `json:"risk_level"`
	AssigneeID string `json:"assignee_id"`
	Search     string `json:"search"`
}

// Exception represents a compliance exception.
type Exception struct {
	ID               string   `json:"id"`
	OrganizationID   string   `json:"organization_id"`
	Title            string   `json:"title" validate:"required"`
	Description      string   `json:"description"`
	Type             string   `json:"type"`      // policy, control, regulatory
	Status           string   `json:"status"`    // draft, pending_approval, approved, rejected, expired, revoked
	RiskLevel        string   `json:"risk_level"` // critical, high, medium, low
	Justification    string   `json:"justification"`
	CompensatingControls string `json:"compensating_controls,omitempty"`
	AffectedControlIDs []string `json:"affected_control_ids,omitempty"`
	AffectedPolicyIDs  []string `json:"affected_policy_ids,omitempty"`
	RequestedBy      string   `json:"requested_by"`
	ApprovedBy       string   `json:"approved_by,omitempty"`
	ApprovedAt       string   `json:"approved_at,omitempty"`
	ExpiresAt        string   `json:"expires_at,omitempty"`
	RenewedAt        string   `json:"renewed_at,omitempty"`
	RevokedAt        string   `json:"revoked_at,omitempty"`
	LastReviewedAt   string   `json:"last_reviewed_at,omitempty"`
	NextReviewDate   string   `json:"next_review_date,omitempty"`
	CreatedBy        string   `json:"created_by"`
	CreatedAt        string   `json:"created_at"`
	UpdatedAt        string   `json:"updated_at"`
}

// ExceptionDetail extends Exception with related data.
type ExceptionDetail struct {
	Exception
	Reviews     []ExceptionReview `json:"reviews,omitempty"`
	AuditTrail  []ExceptionAudit  `json:"audit_trail,omitempty"`
}

// ExceptionReview represents a periodic review of an exception.
type ExceptionReview struct {
	ID           string `json:"id"`
	ExceptionID  string `json:"exception_id"`
	ReviewedBy   string `json:"reviewed_by"`
	Status       string `json:"status"` // continued, escalated, revoked
	Comments     string `json:"comments,omitempty"`
	RiskReassessment string `json:"risk_reassessment,omitempty"`
	ReviewedAt   string `json:"reviewed_at"`
}

// ExceptionAudit represents an audit trail entry for an exception.
type ExceptionAudit struct {
	Action    string `json:"action"`
	UserID    string `json:"user_id"`
	Details   string `json:"details,omitempty"`
	Timestamp string `json:"timestamp"`
}

// ExceptionApprovalRequest is the payload for POST /exceptions/{id}/approve.
type ExceptionApprovalRequest struct {
	Comments   string `json:"comments,omitempty"`
	ExpiresAt  string `json:"expires_at,omitempty"`
	Conditions string `json:"conditions,omitempty"`
}

// ExceptionRejectionRequest is the payload for POST /exceptions/{id}/reject.
type ExceptionRejectionRequest struct {
	Reason string `json:"reason" validate:"required"`
}

// ExceptionRevokeRequest is the payload for POST /exceptions/{id}/revoke.
type ExceptionRevokeRequest struct {
	Reason string `json:"reason" validate:"required"`
}

// ExceptionRenewRequest is the payload for POST /exceptions/{id}/renew.
type ExceptionRenewRequest struct {
	NewExpiresAt     string `json:"new_expires_at" validate:"required"`
	Justification    string `json:"justification,omitempty"`
	RiskReassessment string `json:"risk_reassessment,omitempty"`
}

// ExceptionReviewRequest is the payload for POST /exceptions/{id}/review.
type ExceptionReviewRequest struct {
	Status           string `json:"status" validate:"required"` // continued, escalated, revoked
	Comments         string `json:"comments,omitempty"`
	RiskReassessment string `json:"risk_reassessment,omitempty"`
}

// ExceptionDashboard provides exception metrics for an organization.
type ExceptionDashboard struct {
	TotalExceptions    int            `json:"total_exceptions"`
	ActiveExceptions   int            `json:"active_exceptions"`
	PendingApproval    int            `json:"pending_approval"`
	ExpiringIn30Days   int            `json:"expiring_in_30_days"`
	ByStatus           map[string]int `json:"by_status"`
	ByRiskLevel        map[string]int `json:"by_risk_level"`
	ByType             map[string]int `json:"by_type"`
	OverdueReviews     int            `json:"overdue_reviews"`
	AverageApprovalDays float64       `json:"average_approval_days"`
}

// ExceptionImpactAnalysis provides impact analysis for an exception.
type ExceptionImpactAnalysis struct {
	ExceptionID         string           `json:"exception_id"`
	AffectedControls    []AffectedItem   `json:"affected_controls"`
	AffectedPolicies    []AffectedItem   `json:"affected_policies"`
	RiskImpact          string           `json:"risk_impact"`
	ComplianceGaps      []string         `json:"compliance_gaps,omitempty"`
	FrameworksImpacted  []string         `json:"frameworks_impacted,omitempty"`
	OverallRiskIncrease string           `json:"overall_risk_increase"`
	Recommendations     []string         `json:"recommendations,omitempty"`
}

// AffectedItem represents an item affected by an exception.
type AffectedItem struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Type   string `json:"type"`
	Impact string `json:"impact"`
}

// ---------- handler ----------

// ExceptionHandler handles compliance exception management endpoints.
type ExceptionHandler struct {
	svc ExceptionService
}

// NewExceptionHandler creates a new ExceptionHandler with the given service.
func NewExceptionHandler(svc ExceptionService) *ExceptionHandler {
	return &ExceptionHandler{svc: svc}
}

// List handles GET /exceptions.
func (h *ExceptionHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	pagination := parsePagination(r)

	filters := ExceptionFilters{
		Status:     r.URL.Query().Get("status"),
		Type:       r.URL.Query().Get("type"),
		RiskLevel:  r.URL.Query().Get("risk_level"),
		AssigneeID: r.URL.Query().Get("assignee_id"),
		Search:     r.URL.Query().Get("search"),
	}

	exceptions, total, err := h.svc.ListExceptions(r.Context(), orgID, pagination, filters)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list exceptions", err.Error())
		return
	}

	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": exceptions,
		"pagination": models.PaginationResponse{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}

// Create handles POST /exceptions.
func (h *ExceptionHandler) Create(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	var exception Exception
	if err := json.NewDecoder(r.Body).Decode(&exception); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if exception.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required", "")
		return
	}

	if err := h.svc.CreateException(r.Context(), orgID, userID, &exception); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create exception", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, exception)
}

// GetByID handles GET /exceptions/{id}.
func (h *ExceptionHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	exceptionID := chi.URLParam(r, "id")
	if exceptionID == "" {
		writeError(w, http.StatusBadRequest, "Missing exception ID", "")
		return
	}

	detail, err := h.svc.GetException(r.Context(), orgID, exceptionID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Exception not found", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, detail)
}

// Update handles PUT /exceptions/{id}.
func (h *ExceptionHandler) Update(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	exceptionID := chi.URLParam(r, "id")
	if exceptionID == "" {
		writeError(w, http.StatusBadRequest, "Missing exception ID", "")
		return
	}

	var exception Exception
	if err := json.NewDecoder(r.Body).Decode(&exception); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	exception.ID = exceptionID
	exception.OrganizationID = orgID

	if err := h.svc.UpdateException(r.Context(), orgID, &exception); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update exception", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, exception)
}

// Submit handles POST /exceptions/{id}/submit.
func (h *ExceptionHandler) Submit(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	exceptionID := chi.URLParam(r, "id")
	if exceptionID == "" {
		writeError(w, http.StatusBadRequest, "Missing exception ID", "")
		return
	}

	if err := h.svc.SubmitException(r.Context(), orgID, userID, exceptionID); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to submit exception for approval", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Exception submitted for approval"})
}

// Approve handles POST /exceptions/{id}/approve.
func (h *ExceptionHandler) Approve(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	exceptionID := chi.URLParam(r, "id")
	if exceptionID == "" {
		writeError(w, http.StatusBadRequest, "Missing exception ID", "")
		return
	}

	var req ExceptionApprovalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if err := h.svc.ApproveException(r.Context(), orgID, userID, exceptionID, &req); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to approve exception", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Exception approved"})
}

// Reject handles POST /exceptions/{id}/reject.
func (h *ExceptionHandler) Reject(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	exceptionID := chi.URLParam(r, "id")
	if exceptionID == "" {
		writeError(w, http.StatusBadRequest, "Missing exception ID", "")
		return
	}

	var req ExceptionRejectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if req.Reason == "" {
		writeError(w, http.StatusBadRequest, "reason is required", "")
		return
	}

	if err := h.svc.RejectException(r.Context(), orgID, userID, exceptionID, &req); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to reject exception", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Exception rejected"})
}

// Revoke handles POST /exceptions/{id}/revoke.
func (h *ExceptionHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	exceptionID := chi.URLParam(r, "id")
	if exceptionID == "" {
		writeError(w, http.StatusBadRequest, "Missing exception ID", "")
		return
	}

	var req ExceptionRevokeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if req.Reason == "" {
		writeError(w, http.StatusBadRequest, "reason is required", "")
		return
	}

	if err := h.svc.RevokeException(r.Context(), orgID, userID, exceptionID, &req); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to revoke exception", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Exception revoked"})
}

// Renew handles POST /exceptions/{id}/renew.
func (h *ExceptionHandler) Renew(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	exceptionID := chi.URLParam(r, "id")
	if exceptionID == "" {
		writeError(w, http.StatusBadRequest, "Missing exception ID", "")
		return
	}

	var req ExceptionRenewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if req.NewExpiresAt == "" {
		writeError(w, http.StatusBadRequest, "new_expires_at is required", "")
		return
	}

	if err := h.svc.RenewException(r.Context(), orgID, userID, exceptionID, &req); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to renew exception", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Exception renewed"})
}

// Review handles POST /exceptions/{id}/review.
func (h *ExceptionHandler) Review(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	exceptionID := chi.URLParam(r, "id")
	if exceptionID == "" {
		writeError(w, http.StatusBadRequest, "Missing exception ID", "")
		return
	}

	var req ExceptionReviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if req.Status == "" {
		writeError(w, http.StatusBadRequest, "status is required", "")
		return
	}

	if err := h.svc.ReviewException(r.Context(), orgID, userID, exceptionID, &req); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to review exception", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Exception reviewed"})
}

// GetDashboard handles GET /exceptions/dashboard.
func (h *ExceptionHandler) GetDashboard(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())

	dashboard, err := h.svc.GetDashboard(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get exception dashboard", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, dashboard)
}

// GetExpiring handles GET /exceptions/expiring.
func (h *ExceptionHandler) GetExpiring(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())

	daysAhead := 30
	if d := r.URL.Query().Get("days"); d != "" {
		var parsed int
		if _, err := json.Number(d).Int64(); err == nil {
			parsed = int(mustParseInt(d))
			if parsed > 0 {
				daysAhead = parsed
			}
		}
	}

	exceptions, err := h.svc.GetExpiring(r.Context(), orgID, daysAhead)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get expiring exceptions", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":       exceptions,
		"days_ahead": daysAhead,
	})
}

// GetImpactAnalysis handles GET /exceptions/impact/{id}.
func (h *ExceptionHandler) GetImpactAnalysis(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	exceptionID := chi.URLParam(r, "id")
	if exceptionID == "" {
		writeError(w, http.StatusBadRequest, "Missing exception ID", "")
		return
	}

	analysis, err := h.svc.GetImpactAnalysis(r.Context(), orgID, exceptionID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Impact analysis not found", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, analysis)
}

// mustParseInt parses a string to int64; returns 0 on failure.
func mustParseInt(s string) int64 {
	n, _ := json.Number(s).Int64()
	return n
}

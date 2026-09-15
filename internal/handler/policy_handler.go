package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/complianceforge/platform/internal/middleware"
	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/service"
)

type PolicyService interface {
	Create(context.Context, string, string, models.PolicyCreateInput) (*models.Policy, error)
	GetByID(context.Context, string, string) (*models.Policy, error)
	Update(context.Context, string, string, models.PolicyPatch) (*models.Policy, error)
	AssignOwner(context.Context, string, string, *string, *string) (*models.Policy, error)
	Delete(context.Context, string, string) error
	List(context.Context, string, models.PolicyListFilter) ([]models.Policy, int, error)
	ListCategories(context.Context, string) ([]models.PolicyCategory, error)
	CreateVersion(context.Context, string, string, string, models.PolicyVersionInput) (*models.PolicyVersion, error)
	GetVersion(context.Context, string, string, string) (*models.PolicyVersion, error)
	ListVersions(context.Context, string, string, models.PaginationRequest) ([]models.PolicyVersion, int, error)
	SubmitForApproval(context.Context, string, string, string, models.PolicySubmitInput) (*models.PolicyApprovalWorkflow, error)
	GetActiveApproval(context.Context, string, string) (*models.PolicyApprovalWorkflow, error)
	DecideApproval(context.Context, string, string, string, string, models.PolicyApprovalDecisionInput) (*models.PolicyApprovalWorkflow, error)
	Publish(context.Context, string, string, string) (*models.Policy, error)
	CreateReview(context.Context, string, string, models.PolicyReviewInput) (*models.PolicyReview, error)
	GetReview(context.Context, string, string, string) (*models.PolicyReview, error)
	UpdateReview(context.Context, string, string, string, models.PolicyReviewPatch) (*models.PolicyReview, error)
	ListReviews(context.Context, string, string, models.PaginationRequest) ([]models.PolicyReview, int, error)
	Acknowledge(context.Context, string, string, string, string, models.PolicyAttestationInput) (*models.PolicyAttestation, error)
	ListAttestations(context.Context, string, string, models.PaginationRequest) ([]models.PolicyAttestation, int, error)
	CreateException(context.Context, string, string, string, models.PolicyExceptionInput) (*models.PolicyException, error)
	GetException(context.Context, string, string, string) (*models.PolicyException, error)
	DecideException(context.Context, string, string, string, string, models.PolicyExceptionDecisionInput) (*models.PolicyException, error)
	ListExceptions(context.Context, string, string, models.PaginationRequest) ([]models.PolicyException, int, error)
}

type PolicyHandler struct{ service PolicyService }

func NewPolicyHandler(service PolicyService) *PolicyHandler { return &PolicyHandler{service: service} }
func (h *PolicyHandler) Ready() bool                        { return h != nil && h.service != nil }

func (h *PolicyHandler) Create(w http.ResponseWriter, r *http.Request) {
	var input models.PolicyCreateInput
	if err := decodePolicyJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := h.service.Create(r.Context(), policyOrgID(r), policyUserID(r), input)
	if err != nil {
		writePolicyError(w, err, "Failed to create policy")
		return
	}
	writeClassifiedJSON(w, r, http.StatusCreated, "policies", item)
}

func (h *PolicyHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	item, err := h.service.GetByID(r.Context(), policyOrgID(r), chi.URLParam(r, "id"))
	if err != nil {
		writePolicyError(w, err, "Failed to get policy")
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "policies", item)
}

func (h *PolicyHandler) Update(w http.ResponseWriter, r *http.Request) {
	var patch models.PolicyPatch
	if err := decodePolicyJSON(w, r, &patch); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := h.service.Update(r.Context(), policyOrgID(r), chi.URLParam(r, "id"), patch)
	if err != nil {
		writePolicyError(w, err, "Failed to update policy")
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "policies", item)
}

func (h *PolicyHandler) AssignOwner(w http.ResponseWriter, r *http.Request) {
	var input struct {
		OwnerUserID    *string `json:"owner_user_id"`
		ApproverUserID *string `json:"approver_user_id"`
	}
	if err := decodePolicyJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := h.service.AssignOwner(r.Context(), policyOrgID(r), chi.URLParam(r, "id"), input.OwnerUserID, input.ApproverUserID)
	if err != nil {
		writePolicyError(w, err, "Failed to assign policy ownership")
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "policies", item)
}

func (h *PolicyHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.service.Delete(r.Context(), policyOrgID(r), chi.URLParam(r, "id")); err != nil {
		writePolicyError(w, err, "Failed to delete policy")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *PolicyHandler) List(w http.ResponseWriter, r *http.Request) {
	filter := models.PolicyListFilter{
		PaginationRequest: parsePagination(r),
		Status:            r.URL.Query().Get("status"),
		Classification:    r.URL.Query().Get("classification"),
		CategoryID:        r.URL.Query().Get("category_id"),
		OwnerUserID:       r.URL.Query().Get("owner_user_id"),
		Search:            r.URL.Query().Get("search"),
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("due_before")); raw != "" {
		date, err := time.Parse("2006-01-02", raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Invalid due_before filter", err.Error())
			return
		}
		filter.DueBefore = &date
	}
	items, total, err := h.service.List(r.Context(), policyOrgID(r), filter)
	if err != nil {
		writePolicyError(w, err, "Failed to list policies")
		return
	}
	writePolicyPaginated(w, r, items, total, filter.PaginationRequest)
}

func (h *PolicyHandler) GetDueForReview(w http.ResponseWriter, r *http.Request) {
	cutoff := time.Now().UTC().AddDate(0, 1, 0)
	pagination := parsePagination(r)
	items, total, err := h.service.List(r.Context(), policyOrgID(r), models.PolicyListFilter{PaginationRequest: pagination, DueBefore: &cutoff})
	if err != nil {
		writePolicyError(w, err, "Failed to list policies due for review")
		return
	}
	writePolicyPaginated(w, r, items, total, pagination)
}

func (h *PolicyHandler) ListCategories(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.ListCategories(r.Context(), policyOrgID(r))
	if err != nil {
		writePolicyError(w, err, "Failed to list policy categories")
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "policies", map[string]any{"data": items})
}

func (h *PolicyHandler) CreateVersion(w http.ResponseWriter, r *http.Request) {
	var input models.PolicyVersionInput
	if err := decodePolicyJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := h.service.CreateVersion(r.Context(), policyOrgID(r), chi.URLParam(r, "id"), policyUserID(r), input)
	if err != nil {
		writePolicyError(w, err, "Failed to create policy version")
		return
	}
	writeClassifiedJSON(w, r, http.StatusCreated, "policies", item)
}

func (h *PolicyHandler) GetVersion(w http.ResponseWriter, r *http.Request) {
	item, err := h.service.GetVersion(r.Context(), policyOrgID(r), chi.URLParam(r, "id"), chi.URLParam(r, "versionID"))
	if err != nil {
		writePolicyError(w, err, "Failed to get policy version")
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "policies", item)
}

func (h *PolicyHandler) ListVersions(w http.ResponseWriter, r *http.Request) {
	p := parsePagination(r)
	items, total, err := h.service.ListVersions(r.Context(), policyOrgID(r), chi.URLParam(r, "id"), p)
	if err != nil {
		writePolicyError(w, err, "Failed to list policy versions")
		return
	}
	writePolicyPaginated(w, r, items, total, p)
}

func (h *PolicyHandler) SubmitForApproval(w http.ResponseWriter, r *http.Request) {
	var input models.PolicySubmitInput
	if err := decodePolicyJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := h.service.SubmitForApproval(r.Context(), policyOrgID(r), chi.URLParam(r, "id"), policyUserID(r), input)
	if err != nil {
		writePolicyError(w, err, "Failed to submit policy for approval")
		return
	}
	writeClassifiedJSON(w, r, http.StatusCreated, "policies", item)
}

// SubmitForReview preserves the old route name while using the canonical
// approval workflow instead of directly mutating a status field.
func (h *PolicyHandler) SubmitForReview(w http.ResponseWriter, r *http.Request) {
	h.SubmitForApproval(w, r)
}

func (h *PolicyHandler) GetActiveApproval(w http.ResponseWriter, r *http.Request) {
	item, err := h.service.GetActiveApproval(r.Context(), policyOrgID(r), chi.URLParam(r, "id"))
	if err != nil {
		writePolicyError(w, err, "Failed to get policy approval workflow")
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "policies", item)
}

func (h *PolicyHandler) DecideApproval(w http.ResponseWriter, r *http.Request) {
	var input models.PolicyApprovalDecisionInput
	if err := decodePolicyJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := h.service.DecideApproval(r.Context(), policyOrgID(r), chi.URLParam(r, "id"), policyUserID(r), middleware.GetRoleFromContext(r.Context()), input)
	if err != nil {
		writePolicyError(w, err, "Failed to decide policy approval")
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "policies", item)
}

// Approve is the legacy alias. Its request body is still explicit and audited.
func (h *PolicyHandler) Approve(w http.ResponseWriter, r *http.Request) {
	var input models.PolicyApprovalDecisionInput
	if err := decodePolicyJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	if strings.TrimSpace(input.Decision) == "" {
		input.Decision = "approve"
	}
	item, err := h.service.DecideApproval(r.Context(), policyOrgID(r), chi.URLParam(r, "id"), policyUserID(r), middleware.GetRoleFromContext(r.Context()), input)
	if err != nil {
		writePolicyError(w, err, "Failed to approve policy")
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "policies", item)
}

func (h *PolicyHandler) Publish(w http.ResponseWriter, r *http.Request) {
	item, err := h.service.Publish(r.Context(), policyOrgID(r), chi.URLParam(r, "id"), policyUserID(r))
	if err != nil {
		writePolicyError(w, err, "Failed to publish policy")
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "policies", item)
}

func (h *PolicyHandler) CreateReview(w http.ResponseWriter, r *http.Request) {
	var input models.PolicyReviewInput
	if err := decodePolicyJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := h.service.CreateReview(r.Context(), policyOrgID(r), chi.URLParam(r, "id"), input)
	if err != nil {
		writePolicyError(w, err, "Failed to create policy review")
		return
	}
	writeClassifiedJSON(w, r, http.StatusCreated, "policies", item)
}

func (h *PolicyHandler) GetReview(w http.ResponseWriter, r *http.Request) {
	item, err := h.service.GetReview(r.Context(), policyOrgID(r), chi.URLParam(r, "id"), chi.URLParam(r, "reviewID"))
	if err != nil {
		writePolicyError(w, err, "Failed to get policy review")
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "policies", item)
}

func (h *PolicyHandler) UpdateReview(w http.ResponseWriter, r *http.Request) {
	var patch models.PolicyReviewPatch
	if err := decodePolicyJSON(w, r, &patch); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := h.service.UpdateReview(r.Context(), policyOrgID(r), chi.URLParam(r, "id"), chi.URLParam(r, "reviewID"), patch)
	if err != nil {
		writePolicyError(w, err, "Failed to update policy review")
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "policies", item)
}

func (h *PolicyHandler) ListReviews(w http.ResponseWriter, r *http.Request) {
	p := parsePagination(r)
	items, total, err := h.service.ListReviews(r.Context(), policyOrgID(r), chi.URLParam(r, "id"), p)
	if err != nil {
		writePolicyError(w, err, "Failed to list policy reviews")
		return
	}
	writePolicyPaginated(w, r, items, total, p)
}

func (h *PolicyHandler) Acknowledge(w http.ResponseWriter, r *http.Request) {
	var input models.PolicyAttestationInput
	if err := decodePolicyJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := h.service.Acknowledge(r.Context(), policyOrgID(r), chi.URLParam(r, "id"), policyUserID(r), policyClientIP(r), input)
	if err != nil {
		writePolicyError(w, err, "Failed to acknowledge policy")
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "policies", item)
}

func (h *PolicyHandler) ListAttestations(w http.ResponseWriter, r *http.Request) {
	p := parsePagination(r)
	items, total, err := h.service.ListAttestations(r.Context(), policyOrgID(r), chi.URLParam(r, "id"), p)
	if err != nil {
		writePolicyError(w, err, "Failed to list policy attestations")
		return
	}
	writePolicyPaginated(w, r, items, total, p)
}

func (h *PolicyHandler) CreateException(w http.ResponseWriter, r *http.Request) {
	var input models.PolicyExceptionInput
	if err := decodePolicyJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := h.service.CreateException(r.Context(), policyOrgID(r), chi.URLParam(r, "id"), policyUserID(r), input)
	if err != nil {
		writePolicyError(w, err, "Failed to request policy exception")
		return
	}
	writeClassifiedJSON(w, r, http.StatusCreated, "policies", item)
}

func (h *PolicyHandler) GetException(w http.ResponseWriter, r *http.Request) {
	item, err := h.service.GetException(r.Context(), policyOrgID(r), chi.URLParam(r, "id"), chi.URLParam(r, "exceptionID"))
	if err != nil {
		writePolicyError(w, err, "Failed to get policy exception")
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "policies", item)
}

func (h *PolicyHandler) DecideException(w http.ResponseWriter, r *http.Request) {
	var input models.PolicyExceptionDecisionInput
	if err := decodePolicyJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := h.service.DecideException(r.Context(), policyOrgID(r), chi.URLParam(r, "id"), chi.URLParam(r, "exceptionID"), policyUserID(r), input)
	if err != nil {
		writePolicyError(w, err, "Failed to decide policy exception")
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "policies", item)
}

func (h *PolicyHandler) ListExceptions(w http.ResponseWriter, r *http.Request) {
	p := parsePagination(r)
	items, total, err := h.service.ListExceptions(r.Context(), policyOrgID(r), chi.URLParam(r, "id"), p)
	if err != nil {
		writePolicyError(w, err, "Failed to list policy exceptions")
		return
	}
	writePolicyPaginated(w, r, items, total, p)
}

func policyOrgID(r *http.Request) string  { return middleware.GetOrgIDFromContext(r.Context()) }
func policyUserID(r *http.Request) string { return middleware.GetUserIDFromContext(r.Context()) }

func policyClientIP(r *http.Request) string {
	value := strings.TrimSpace(r.RemoteAddr)
	if host, _, err := net.SplitHostPort(value); err == nil {
		return host
	}
	return value
}

func decodePolicyJSON(w http.ResponseWriter, r *http.Request, destination any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("request body must contain one JSON object")
		}
		return err
	}
	return nil
}

func writePolicyPaginated(w http.ResponseWriter, r *http.Request, data any, total int, p models.PaginationRequest) {
	if p.Page < 1 {
		p.Page = 1
	}
	if p.PageSize < 1 {
		p.PageSize = 20
	}
	if p.PageSize > 100 {
		p.PageSize = 100
	}
	writeClassifiedPaginated(w, r, "policies", data, total, p)
}

func writePolicyError(w http.ResponseWriter, err error, fallback string) {
	switch {
	case errors.Is(err, service.ErrPolicyInvalid), errors.Is(err, service.ErrPolicyInvalidID):
		writeError(w, http.StatusBadRequest, "Invalid policy request", err.Error())
	case errors.Is(err, service.ErrPolicyNotFound), errors.Is(err, service.ErrPolicyVersionNotFound),
		errors.Is(err, service.ErrPolicyWorkflowNotFound), errors.Is(err, service.ErrPolicyReviewNotFound),
		errors.Is(err, service.ErrPolicyExceptionNotFound):
		writeError(w, http.StatusNotFound, "Policy resource not found", err.Error())
	case errors.Is(err, service.ErrPolicyApprovalForbidden):
		writeError(w, http.StatusForbidden, "Policy approval forbidden", err.Error())
	case errors.Is(err, service.ErrPolicyConflict), errors.Is(err, service.ErrPolicyInvalidReference),
		errors.Is(err, service.ErrPolicyInvalidTransition):
		writeError(w, http.StatusConflict, "Policy request conflicts with current state", err.Error())
	default:
		if strings.TrimSpace(fallback) == "" {
			fallback = "Policy request failed"
		}
		writeError(w, http.StatusInternalServerError, fallback, fmt.Sprintf("%v", err))
	}
}

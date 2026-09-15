package handler

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/repository"
	"github.com/complianceforge/platform/internal/service"
)

// Explicit service/store contract keeps the mounted surface compile-checked.
type AccessGovernanceHandlerService interface {
	repository.AccessGovernanceRepository
}
type AccessGovernanceHandler struct {
	service AccessGovernanceHandlerService
}

func NewAccessGovernanceHandler(s AccessGovernanceHandlerService) *AccessGovernanceHandler {
	return &AccessGovernanceHandler{service: s}
}
func (h *AccessGovernanceHandler) Ready() bool { return h != nil && h.service != nil }
func writeGovernanceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, service.ErrGovernanceInvalid):
		writeError(w, 400, "Invalid governance request", "Check identifiers, bounds, reason and lifecycle state")
	case errors.Is(err, service.ErrGovernanceNotFound):
		writeError(w, 404, "Governance resource not found", "")
	case errors.Is(err, service.ErrGovernanceConflict):
		writeError(w, 409, "Governance conflict", "Refresh the snapshot/version; request IDs cannot be reused for a different decision")
	case errors.Is(err, service.ErrGovernanceSeparation):
		writeError(w, 403, "Independent reviewer required", "The subject, sponsor and approver cannot certify their own access")
	case errors.Is(err, service.ErrLastTenantAdministrator):
		writeError(w, 409, "Administrative access safeguard", "Retain an independent permanent administrator")
	default:
		log.Error().Err(err).Str("path", r.URL.Path).Msg("access governance operation failed")
		writeError(w, 500, "Governance operation failed", "")
	}
}
func governanceBody(w http.ResponseWriter, r *http.Request, input any) bool {
	if err := decodeAuditJSON(w, r, input); err != nil {
		writeError(w, 400, "Invalid request body", err.Error())
		return false
	}
	return true
}
func governanceResult(w http.ResponseWriter, r *http.Request, status int, result any, err error) {
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	writeClassifiedJSON(w, r, status, "settings", result)
}
func governanceList(w http.ResponseWriter, r *http.Request, result any, total int, err error) {
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	writeClassifiedPaginated(w, r, "settings", result, total, normalizedHandlerPagination(parsePagination(r)))
}
func (h *AccessGovernanceHandler) CreateCampaign(w http.ResponseWriter, r *http.Request) {
	var in models.AccessReviewCampaignInput
	if !governanceBody(w, r, &in) {
		return
	}
	v, err := h.service.CreateCampaign(r.Context(), accessAdminOrgID(r), accessAdminActorID(r), in)
	governanceResult(w, r, 201, v, err)
}
func (h *AccessGovernanceHandler) ListCampaigns(w http.ResponseWriter, r *http.Request) {
	v, n, err := h.service.ListCampaigns(r.Context(), accessAdminOrgID(r), parsePagination(r))
	governanceList(w, r, v, n, err)
}
func (h *AccessGovernanceHandler) GetCampaign(w http.ResponseWriter, r *http.Request) {
	v, err := h.service.GetCampaign(r.Context(), accessAdminOrgID(r), chi.URLParam(r, "id"))
	governanceResult(w, r, 200, v, err)
}
func (h *AccessGovernanceHandler) ListItems(w http.ResponseWriter, r *http.Request) {
	v, n, err := h.service.ListItems(r.Context(), accessAdminOrgID(r), chi.URLParam(r, "id"), parsePagination(r))
	governanceList(w, r, v, n, err)
}
func (h *AccessGovernanceHandler) DecideItem(w http.ResponseWriter, r *http.Request) {
	var in models.AccessReviewDecisionInput
	if !governanceBody(w, r, &in) {
		return
	}
	v, err := h.service.DecideItem(r.Context(), accessAdminOrgID(r), chi.URLParam(r, "id"), chi.URLParam(r, "itemID"), accessAdminActorID(r), in)
	governanceResult(w, r, 200, v, err)
}
func (h *AccessGovernanceHandler) CompleteCampaign(w http.ResponseWriter, r *http.Request) {
	h.transitionCampaign(w, r, "completed")
}
func (h *AccessGovernanceHandler) CancelCampaign(w http.ResponseWriter, r *http.Request) {
	h.transitionCampaign(w, r, "cancelled")
}
func (h *AccessGovernanceHandler) transitionCampaign(w http.ResponseWriter, r *http.Request, status string) {
	var in models.AccessGovernanceTransitionInput
	if !governanceBody(w, r, &in) {
		return
	}
	v, err := h.service.TransitionCampaign(r.Context(), accessAdminOrgID(r), chi.URLParam(r, "id"), accessAdminActorID(r), status, in)
	governanceResult(w, r, 200, v, err)
}
func (h *AccessGovernanceHandler) CreateSoDRule(w http.ResponseWriter, r *http.Request) {
	var in models.AccessSoDRuleInput
	if !governanceBody(w, r, &in) {
		return
	}
	v, err := h.service.CreateSoDRule(r.Context(), accessAdminOrgID(r), accessAdminActorID(r), in)
	governanceResult(w, r, 201, v, err)
}
func (h *AccessGovernanceHandler) ListSoDRules(w http.ResponseWriter, r *http.Request) {
	v, n, err := h.service.ListSoDRules(r.Context(), accessAdminOrgID(r), parsePagination(r))
	governanceList(w, r, v, n, err)
}
func (h *AccessGovernanceHandler) DisableSoDRule(w http.ResponseWriter, r *http.Request) {
	var in models.AccessGovernanceTransitionInput
	if !governanceBody(w, r, &in) {
		return
	}
	v, err := h.service.DisableSoDRule(r.Context(), accessAdminOrgID(r), chi.URLParam(r, "id"), accessAdminActorID(r), in)
	governanceResult(w, r, 200, v, err)
}
func (h *AccessGovernanceHandler) RequestException(w http.ResponseWriter, r *http.Request) {
	var in models.AccessSoDExceptionInput
	if !governanceBody(w, r, &in) {
		return
	}
	v, err := h.service.RequestException(r.Context(), accessAdminOrgID(r), chi.URLParam(r, "id"), accessAdminActorID(r), in)
	governanceResult(w, r, 201, v, err)
}
func (h *AccessGovernanceHandler) ApproveException(w http.ResponseWriter, r *http.Request) {
	h.decideException(w, r, "approved")
}
func (h *AccessGovernanceHandler) RejectException(w http.ResponseWriter, r *http.Request) {
	h.decideException(w, r, "rejected")
}
func (h *AccessGovernanceHandler) RevokeException(w http.ResponseWriter, r *http.Request) {
	h.decideException(w, r, "revoked")
}
func (h *AccessGovernanceHandler) decideException(w http.ResponseWriter, r *http.Request, status string) {
	var in models.AccessGovernanceTransitionInput
	if !governanceBody(w, r, &in) {
		return
	}
	v, err := h.service.DecideException(r.Context(), accessAdminOrgID(r), chi.URLParam(r, "id"), accessAdminActorID(r), status, in)
	governanceResult(w, r, 200, v, err)
}
func (h *AccessGovernanceHandler) ListExceptions(w http.ResponseWriter, r *http.Request) {
	v, n, err := h.service.ListExceptions(r.Context(), accessAdminOrgID(r), parsePagination(r))
	governanceList(w, r, v, n, err)
}
func (h *AccessGovernanceHandler) ListViolations(w http.ResponseWriter, r *http.Request) {
	v, n, err := h.service.ListViolations(r.Context(), accessAdminOrgID(r), parsePagination(r))
	governanceList(w, r, v, n, err)
}
func (h *AccessGovernanceHandler) ListEvents(w http.ResponseWriter, r *http.Request) {
	v, n, err := h.service.ListEvents(r.Context(), accessAdminOrgID(r), parsePagination(r))
	governanceList(w, r, v, n, err)
}
func (h *AccessGovernanceHandler) SetAssignmentWindow(w http.ResponseWriter, r *http.Request) {
	var in models.ManagedRoleWindowInput
	if !governanceBody(w, r, &in) {
		return
	}
	v, err := h.service.SetAssignmentWindow(r.Context(), accessAdminOrgID(r), chi.URLParam(r, "id"), chi.URLParam(r, "userID"), accessAdminActorID(r), in)
	governanceResult(w, r, 200, v, err)
}

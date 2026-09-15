package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"github.com/complianceforge/platform/internal/apiresponse"
	"github.com/complianceforge/platform/internal/authz"
	"github.com/complianceforge/platform/internal/middleware"
	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/service"
)

const maxAccessPolicyRequestBytes = 256 << 10

// PolicyAccessHandlerService is the typed administration/evidence boundary for
// policy-based access controls. It deliberately contains no interface{} payloads.
type PolicyAccessHandlerService interface {
	ListPolicies(context.Context, string, models.AccessPolicyListFilter) ([]models.AccessPolicy, int, error)
	GetPolicy(context.Context, string, string) (*models.AccessPolicy, error)
	CreatePolicy(context.Context, string, string, string, models.AccessPolicyInput) (*models.AccessPolicy, error)
	UpdatePolicy(context.Context, string, string, string, string, models.AccessPolicyInput) (*models.AccessPolicy, error)
	DeletePolicy(context.Context, string, string, string, string, int64, string) error
	ListAssignments(context.Context, string, string) ([]models.AccessPolicyAssignment, error)
	CreateAssignment(context.Context, string, string, string, string, models.AccessPolicyAssignmentInput) (*models.AccessPolicyAssignment, error)
	RemoveAssignment(context.Context, string, string, string, string, string, string) error
	ListFieldPermissions(context.Context, string, string, string) ([]models.AccessFieldPermission, error)
	UpsertFieldPermission(context.Context, string, string, string, string, models.AccessFieldPermissionInput) (*models.AccessFieldPermission, error)
	DeleteFieldPermission(context.Context, string, string, string, string, string, int64, string) error
	ListObjectGrants(context.Context, string, models.AccessObjectGrantFilter) ([]models.AccessObjectGrant, int, error)
	CreateObjectGrant(context.Context, string, string, string, models.AccessObjectGrantInput) (*models.AccessObjectGrant, error)
	DecideObjectGrant(context.Context, string, string, string, string, models.AccessObjectGrantDecisionInput) (*models.AccessObjectGrant, error)
	RevokeObjectGrant(context.Context, string, string, string, string, models.AccessObjectGrantRevocationInput) (*models.AccessObjectGrant, error)
	ListDecisionEvidence(context.Context, string, models.AccessDecisionEvidenceFilter) ([]models.AccessDecisionEvidence, int, error)
	CertifyPolicy(context.Context, string, string, string, string, models.AccessPolicyCertificationInput) (*models.AccessPolicyCertification, error)
	ListPolicyCertifications(context.Context, string, string, models.PaginationRequest) ([]models.AccessPolicyCertification, int, error)
}

// AccessHandler exposes tenant policy administration. The separate authorizer
// is used only by the administrator simulation endpoint; normal routes are
// already enforced by the router's composite authorization middleware.
type AccessHandler struct {
	service    PolicyAccessHandlerService
	authorizer authz.Authorizer
}

func NewAccessHandler(service PolicyAccessHandlerService, authorizer authz.Authorizer) *AccessHandler {
	return &AccessHandler{service: service, authorizer: authorizer}
}

func (h *AccessHandler) Ready() bool { return h != nil && h.service != nil && h.authorizer != nil }

func (h *AccessHandler) ListPolicies(w http.ResponseWriter, r *http.Request) {
	active, err := optionalAccessBool(r, "active")
	if err != nil {
		writeAccessError(w, r, err)
		return
	}
	filter := models.AccessPolicyListFilter{PaginationRequest: parsePagination(r),
		Search: strings.TrimSpace(r.URL.Query().Get("search")), ResourceType: strings.TrimSpace(r.URL.Query().Get("resource_type")),
		Effect: models.AccessPolicyEffect(strings.TrimSpace(r.URL.Query().Get("effect"))), Active: active}
	items, total, err := h.service.ListPolicies(r.Context(), accessOrganizationID(r), filter)
	if err != nil {
		writeAccessError(w, r, err)
		return
	}
	writeClassifiedPaginated(w, r, "settings", items, total, normalizedHandlerPagination(filter.PaginationRequest))
}

func (h *AccessHandler) GetPolicy(w http.ResponseWriter, r *http.Request) {
	item, err := h.service.GetPolicy(r.Context(), accessOrganizationID(r), chi.URLParam(r, "id"))
	if err != nil {
		writeAccessError(w, r, err)
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "settings", map[string]any{"data": item})
}

func (h *AccessHandler) CreatePolicy(w http.ResponseWriter, r *http.Request) {
	var input models.AccessPolicyInput
	if !decodeAccessJSON(w, r, &input) {
		return
	}
	item, err := h.service.CreatePolicy(r.Context(), accessOrganizationID(r), accessActorID(r), accessRequestID(r), input)
	if err != nil {
		writeAccessError(w, r, err)
		return
	}
	writeClassifiedJSON(w, r, http.StatusCreated, "settings", map[string]any{"data": item})
}

func (h *AccessHandler) UpdatePolicy(w http.ResponseWriter, r *http.Request) {
	var input models.AccessPolicyInput
	if !decodeAccessJSON(w, r, &input) {
		return
	}
	item, err := h.service.UpdatePolicy(r.Context(), accessOrganizationID(r), chi.URLParam(r, "id"), accessActorID(r), accessRequestID(r), input)
	if err != nil {
		writeAccessError(w, r, err)
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "settings", map[string]any{"data": item})
}

func (h *AccessHandler) DeletePolicy(w http.ResponseWriter, r *http.Request) {
	expectedVersion, reason, ok := accessVersionAndReason(w, r)
	if !ok {
		return
	}
	if err := h.service.DeletePolicy(r.Context(), accessOrganizationID(r), chi.URLParam(r, "id"), accessActorID(r), accessRequestID(r), expectedVersion, reason); err != nil {
		writeAccessError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AccessHandler) ListAssignments(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.ListAssignments(r.Context(), accessOrganizationID(r), chi.URLParam(r, "id"))
	if err != nil {
		writeAccessError(w, r, err)
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "settings", map[string]any{"data": items})
}

func (h *AccessHandler) AssignPolicy(w http.ResponseWriter, r *http.Request) {
	var input models.AccessPolicyAssignmentInput
	if !decodeAccessJSON(w, r, &input) {
		return
	}
	item, err := h.service.CreateAssignment(r.Context(), accessOrganizationID(r), chi.URLParam(r, "id"), accessActorID(r), accessRequestID(r), input)
	if err != nil {
		writeAccessError(w, r, err)
		return
	}
	writeClassifiedJSON(w, r, http.StatusCreated, "settings", map[string]any{"data": item})
}

func (h *AccessHandler) RemoveAssignment(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Reason string `json:"reason"`
	}
	if !decodeAccessJSON(w, r, &input) {
		return
	}
	if err := h.service.RemoveAssignment(r.Context(), accessOrganizationID(r), chi.URLParam(r, "id"), chi.URLParam(r, "assignmentId"), accessActorID(r), accessRequestID(r), input.Reason); err != nil {
		writeAccessError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AccessHandler) ListPolicyFieldPermissions(w http.ResponseWriter, r *http.Request) {
	h.listFieldPermissions(w, r, chi.URLParam(r, "id"))
}

func (h *AccessHandler) GetFieldPermissions(w http.ResponseWriter, r *http.Request) {
	h.listFieldPermissions(w, r, "")
}

func (h *AccessHandler) listFieldPermissions(w http.ResponseWriter, r *http.Request, policyID string) {
	items, err := h.service.ListFieldPermissions(r.Context(), accessOrganizationID(r), policyID, strings.TrimSpace(r.URL.Query().Get("resource_type")))
	if err != nil {
		writeAccessError(w, r, err)
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "settings", map[string]any{"data": items})
}

func (h *AccessHandler) UpsertFieldPermission(w http.ResponseWriter, r *http.Request) {
	var input models.AccessFieldPermissionInput
	if !decodeAccessJSON(w, r, &input) {
		return
	}
	item, err := h.service.UpsertFieldPermission(r.Context(), accessOrganizationID(r), chi.URLParam(r, "id"), accessActorID(r), accessRequestID(r), input)
	if err != nil {
		writeAccessError(w, r, err)
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "settings", map[string]any{"data": item})
}

func (h *AccessHandler) DeleteFieldPermission(w http.ResponseWriter, r *http.Request) {
	expectedVersion, reason, ok := accessVersionAndReason(w, r)
	if !ok {
		return
	}
	if err := h.service.DeleteFieldPermission(r.Context(), accessOrganizationID(r), chi.URLParam(r, "id"), chi.URLParam(r, "fieldPermissionId"), accessActorID(r), accessRequestID(r), expectedVersion, reason); err != nil {
		writeAccessError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AccessHandler) ListObjectGrants(w http.ResponseWriter, r *http.Request) {
	filter := models.AccessObjectGrantFilter{PaginationRequest: parsePagination(r), SubjectID: strings.TrimSpace(r.URL.Query().Get("subject_id")),
		ResourceType: strings.TrimSpace(r.URL.Query().Get("resource_type")), ResourceID: strings.TrimSpace(r.URL.Query().Get("resource_id")),
		Status: models.AccessObjectGrantStatus(strings.TrimSpace(r.URL.Query().Get("status")))}
	if activeAt := strings.TrimSpace(r.URL.Query().Get("active_at")); activeAt != "" {
		parsed, parseErr := time.Parse(time.RFC3339, activeAt)
		if parseErr != nil {
			writeAccessError(w, r, service.ErrInvalidAccessPolicy)
			return
		}
		filter.ActiveAt = &parsed
	}
	items, total, err := h.service.ListObjectGrants(r.Context(), accessOrganizationID(r), filter)
	if err != nil {
		writeAccessError(w, r, err)
		return
	}
	writeClassifiedPaginated(w, r, "settings", items, total, normalizedHandlerPagination(filter.PaginationRequest))
}

func (h *AccessHandler) CreateObjectGrant(w http.ResponseWriter, r *http.Request) {
	var input models.AccessObjectGrantInput
	if !decodeAccessJSON(w, r, &input) {
		return
	}
	item, err := h.service.CreateObjectGrant(r.Context(), accessOrganizationID(r), accessActorID(r), accessRequestID(r), input)
	if err != nil {
		writeAccessError(w, r, err)
		return
	}
	writeClassifiedJSON(w, r, http.StatusCreated, "settings", map[string]any{"data": item})
}

func (h *AccessHandler) DecideObjectGrant(w http.ResponseWriter, r *http.Request) {
	var input models.AccessObjectGrantDecisionInput
	if !decodeAccessJSON(w, r, &input) {
		return
	}
	item, err := h.service.DecideObjectGrant(r.Context(), accessOrganizationID(r), chi.URLParam(r, "grantId"), accessActorID(r), accessRequestID(r), input)
	if err != nil {
		writeAccessError(w, r, err)
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "settings", map[string]any{"data": item})
}

func (h *AccessHandler) RevokeObjectGrant(w http.ResponseWriter, r *http.Request) {
	var input models.AccessObjectGrantRevocationInput
	if !decodeAccessJSON(w, r, &input) {
		return
	}
	item, err := h.service.RevokeObjectGrant(r.Context(), accessOrganizationID(r), chi.URLParam(r, "grantId"), accessActorID(r), accessRequestID(r), input)
	if err != nil {
		writeAccessError(w, r, err)
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "settings", map[string]any{"data": item})
}

func (h *AccessHandler) ListDecisionEvidence(w http.ResponseWriter, r *http.Request) {
	filter, err := accessEvidenceFilter(r)
	if err != nil {
		writeAccessError(w, r, err)
		return
	}
	items, total, err := h.service.ListDecisionEvidence(r.Context(), accessOrganizationID(r), filter)
	if err != nil {
		writeAccessError(w, r, err)
		return
	}
	writeClassifiedPaginated(w, r, "settings", items, total, normalizedHandlerPagination(filter.PaginationRequest))
}

func (h *AccessHandler) GetAuditLog(w http.ResponseWriter, r *http.Request) {
	h.ListDecisionEvidence(w, r)
}

func (h *AccessHandler) ListPolicyCertifications(w http.ResponseWriter, r *http.Request) {
	pagination := normalizedHandlerPagination(parsePagination(r))
	items, total, err := h.service.ListPolicyCertifications(r.Context(), accessOrganizationID(r), chi.URLParam(r, "id"), pagination)
	if err != nil {
		writeAccessError(w, r, err)
		return
	}
	writeClassifiedPaginated(w, r, "settings", items, total, pagination)
}

func (h *AccessHandler) CertifyPolicy(w http.ResponseWriter, r *http.Request) {
	var input models.AccessPolicyCertificationInput
	if !decodeAccessJSON(w, r, &input) {
		return
	}
	item, err := h.service.CertifyPolicy(r.Context(), accessOrganizationID(r), chi.URLParam(r, "id"), accessActorID(r), accessRequestID(r), input)
	if err != nil {
		writeAccessError(w, r, err)
		return
	}
	writeClassifiedJSON(w, r, http.StatusCreated, "settings", map[string]any{"data": item})
}

type accessEvaluationInput struct {
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id,omitempty"`
	Action       string `json:"action"`
}

func (h *AccessHandler) TestEvaluate(w http.ResponseWriter, r *http.Request) {
	var input accessEvaluationInput
	if !decodeAccessJSON(w, r, &input) {
		return
	}
	decision, err := h.authorizer.Authorize(r.Context(), authz.Request{SubjectID: accessActorID(r), OrganizationID: accessOrganizationID(r),
		Role: middleware.GetRoleFromContext(r.Context()), Resource: strings.TrimSpace(input.ResourceType), ResourceID: strings.TrimSpace(input.ResourceID),
		Action: strings.TrimSpace(input.Action), IPAddress: middleware.GetClientIPFromContext(r.Context()),
		MFAVerified: middleware.GetMFAVerifiedFromContext(r.Context()), Attributes: map[string]any{"request_id": accessRequestID(r)}})
	if err != nil {
		writeAccessError(w, r, err)
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "settings", map[string]any{"data": decision})
}

func accessOrganizationID(r *http.Request) string { return middleware.GetOrgIDFromContext(r.Context()) }
func accessActorID(r *http.Request) string        { return middleware.GetUserIDFromContext(r.Context()) }
func accessRequestID(r *http.Request) string      { return middleware.GetRequestIDFromContext(r.Context()) }

func decodeAccessJSON(w http.ResponseWriter, r *http.Request, destination any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxAccessPolicyRequestBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		writeAccessResponseError(w, r, http.StatusBadRequest, "invalid_access_policy", "Invalid access-policy request", "Provide one valid JSON object")
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeAccessResponseError(w, r, http.StatusBadRequest, "invalid_access_policy", "Invalid access-policy request", "Provide exactly one JSON object")
		return false
	}
	return true
}

func accessVersionAndReason(w http.ResponseWriter, r *http.Request) (int64, string, bool) {
	version, err := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("expected_version")), 10, 64)
	if err != nil || version < 1 {
		writeAccessResponseError(w, r, http.StatusBadRequest, "invalid_access_policy", "Invalid access-policy request", "expected_version must be a positive integer")
		return 0, "", false
	}
	reason := strings.TrimSpace(r.URL.Query().Get("reason"))
	if reason == "" {
		writeAccessResponseError(w, r, http.StatusBadRequest, "invalid_access_policy", "Invalid access-policy request", "reason is required")
		return 0, "", false
	}
	return version, reason, true
}

func optionalAccessBool(r *http.Request, key string) (*bool, error) {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return nil, nil
	}
	parsed, err := strconv.ParseBool(raw)
	if err != nil {
		return nil, service.ErrInvalidAccessPolicy
	}
	return &parsed, nil
}

func accessEvidenceFilter(r *http.Request) (models.AccessDecisionEvidenceFilter, error) {
	filter := models.AccessDecisionEvidenceFilter{PaginationRequest: parsePagination(r), SubjectID: strings.TrimSpace(r.URL.Query().Get("subject_id")),
		ResourceType: strings.TrimSpace(r.URL.Query().Get("resource_type")), ResourceID: strings.TrimSpace(r.URL.Query().Get("resource_id")),
		Action: strings.TrimSpace(r.URL.Query().Get("action")), Decision: strings.TrimSpace(r.URL.Query().Get("decision"))}
	for key, destination := range map[string]**time.Time{"from": &filter.From, "until": &filter.Until} {
		raw := strings.TrimSpace(r.URL.Query().Get(key))
		if raw == "" {
			continue
		}
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return models.AccessDecisionEvidenceFilter{}, service.ErrInvalidAccessPolicy
		}
		*destination = &parsed
	}
	return filter, nil
}

func writeAccessError(w http.ResponseWriter, r *http.Request, err error) {
	requestID := accessRequestID(r)
	switch {
	case errors.Is(err, service.ErrInvalidAccessPolicy):
		writeAccessResponseError(w, r, http.StatusBadRequest, "invalid_access_policy", "Invalid access-policy request", "Review the supplied identifiers, conditions, window, version, and reason")
	case errors.Is(err, service.ErrAccessPolicyNotFound):
		writeAccessResponseError(w, r, http.StatusNotFound, "access_control_not_found", "Access-control resource not found", "")
	case errors.Is(err, service.ErrAccessPolicyConflict):
		writeAccessResponseError(w, r, http.StatusConflict, "access_control_conflict", "Access-control resource changed", "Reload the latest version and retry")
	case errors.Is(err, service.ErrAccessPolicyState):
		writeAccessResponseError(w, r, http.StatusConflict, "access_control_state_conflict", "Access-control state transition is not allowed", "Reload the resource and review its current state")
	case errors.Is(err, service.ErrAccessDecisionUnavailable), errors.Is(err, service.ErrAccessEvidenceUnavailable):
		log.Error().Str("request_id", requestID).Msg("policy access decision failed closed")
		writeAccessResponseError(w, r, http.StatusServiceUnavailable, "authorization_unavailable", "Authorization is temporarily unavailable", "")
	default:
		log.Error().Str("request_id", requestID).Msg("access-policy request failed")
		writeAccessResponseError(w, r, http.StatusInternalServerError, "access_control_failed", "Access-control request failed", "")
	}
}

func writeAccessResponseError(w http.ResponseWriter, r *http.Request, status int, code, message, details string) {
	apiresponse.WriteError(w, status, code, message, details, accessRequestID(r))
}

package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"github.com/complianceforge/platform/internal/middleware"
	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/service"
)

type AccessAdministrationService interface {
	ListPermissions(context.Context, string) ([]models.PermissionGrant, error)
	CreateRole(context.Context, string, string, models.ManagedRoleCreateInput) (*models.ManagedRole, error)
	GetRole(context.Context, string, string) (*models.ManagedRole, error)
	ListRoles(context.Context, string, models.ManagedRoleListFilter) ([]models.ManagedRole, int, error)
	UpdateRole(context.Context, string, string, string, models.ManagedRolePatch) (*models.ManagedRole, error)
	DeleteRole(context.Context, string, string, string, int64) error
	CloneRole(context.Context, string, string, string, models.ManagedRoleCloneInput) (*models.ManagedRole, error)
	PreviewImpact(context.Context, string, string, []models.PermissionGrant) (*models.ManagedRoleImpact, error)
	ListAssignments(context.Context, string, string) ([]models.ManagedRoleAssignment, error)
	AssignRole(context.Context, string, string, string, models.ManagedRoleAssignmentInput) error
	UnassignRole(context.Context, string, string, string, string, models.ManagedRoleUnassignmentInput) error
	ListEvents(context.Context, string, string, models.PaginationRequest) ([]models.RoleChangeEvent, int, error)
}

type AccessAdministrationHandler struct{ service AccessAdministrationService }

func NewAccessAdministrationHandler(service AccessAdministrationService) *AccessAdministrationHandler {
	return &AccessAdministrationHandler{service: service}
}

func (h *AccessAdministrationHandler) Ready() bool { return h != nil && h.service != nil }

func (h *AccessAdministrationHandler) ListPermissions(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.ListPermissions(r.Context(), accessAdminOrgID(r))
	if err != nil {
		writeAccessAdministrationError(w, r, err, "Failed to list permissions")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (h *AccessAdministrationHandler) CreateRole(w http.ResponseWriter, r *http.Request) {
	var input models.ManagedRoleCreateInput
	if err := decodeAuditJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	role, err := h.service.CreateRole(r.Context(), accessAdminOrgID(r), accessAdminActorID(r), input)
	if err != nil {
		writeAccessAdministrationError(w, r, err, "Failed to create role")
		return
	}
	writeJSON(w, http.StatusCreated, role)
}

func (h *AccessAdministrationHandler) GetRole(w http.ResponseWriter, r *http.Request) {
	role, err := h.service.GetRole(r.Context(), accessAdminOrgID(r), chi.URLParam(r, "id"))
	if err != nil {
		writeAccessAdministrationError(w, r, err, "Failed to get role")
		return
	}
	writeJSON(w, http.StatusOK, role)
}

func (h *AccessAdministrationHandler) ListRoles(w http.ResponseWriter, r *http.Request) {
	filter := models.ManagedRoleListFilter{
		PaginationRequest: parsePagination(r),
		Search:            r.URL.Query().Get("search"),
		IncludeSystem:     true,
	}
	if raw, exists := r.URL.Query()["include_system"]; exists {
		value, err := strconv.ParseBool(strings.TrimSpace(raw[len(raw)-1]))
		if err != nil {
			writeError(w, http.StatusBadRequest, "Invalid include_system filter", "include_system must be true or false")
			return
		}
		filter.IncludeSystem = value
	}
	items, total, err := h.service.ListRoles(r.Context(), accessAdminOrgID(r), filter)
	if err != nil {
		writeAccessAdministrationError(w, r, err, "Failed to list roles")
		return
	}
	writePaginated(w, items, total, normalizedHandlerPagination(filter.PaginationRequest))
}

func (h *AccessAdministrationHandler) UpdateRole(w http.ResponseWriter, r *http.Request) {
	var patch models.ManagedRolePatch
	if err := decodeAuditJSON(w, r, &patch); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	role, err := h.service.UpdateRole(r.Context(), accessAdminOrgID(r), chi.URLParam(r, "id"), accessAdminActorID(r), patch)
	if err != nil {
		writeAccessAdministrationError(w, r, err, "Failed to update role")
		return
	}
	writeJSON(w, http.StatusOK, role)
}

func (h *AccessAdministrationHandler) DeleteRole(w http.ResponseWriter, r *http.Request) {
	version, err := optionalPositiveInt64(r.URL.Query().Get("expected_version"))
	if err != nil || version == nil {
		writeError(w, http.StatusBadRequest, "Invalid expected_version", "expected_version is required and must be a positive integer")
		return
	}
	if err := h.service.DeleteRole(r.Context(), accessAdminOrgID(r), chi.URLParam(r, "id"), accessAdminActorID(r), *version); err != nil {
		writeAccessAdministrationError(w, r, err, "Failed to delete role")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AccessAdministrationHandler) CloneRole(w http.ResponseWriter, r *http.Request) {
	var input models.ManagedRoleCloneInput
	if err := decodeAuditJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	role, err := h.service.CloneRole(r.Context(), accessAdminOrgID(r), chi.URLParam(r, "id"), accessAdminActorID(r), input)
	if err != nil {
		writeAccessAdministrationError(w, r, err, "Failed to clone role")
		return
	}
	writeJSON(w, http.StatusCreated, role)
}

func (h *AccessAdministrationHandler) PreviewImpact(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Permissions []models.PermissionGrant `json:"permissions"`
	}
	if err := decodeAuditJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	impact, err := h.service.PreviewImpact(r.Context(), accessAdminOrgID(r), chi.URLParam(r, "id"), input.Permissions)
	if err != nil {
		writeAccessAdministrationError(w, r, err, "Failed to preview role impact")
		return
	}
	writeJSON(w, http.StatusOK, impact)
}

func (h *AccessAdministrationHandler) ListAssignments(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.ListAssignments(r.Context(), accessAdminOrgID(r), chi.URLParam(r, "id"))
	if err != nil {
		writeAccessAdministrationError(w, r, err, "Failed to list role assignments")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (h *AccessAdministrationHandler) AssignRole(w http.ResponseWriter, r *http.Request) {
	var input models.ManagedRoleAssignmentInput
	if err := decodeAuditJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	if err := h.service.AssignRole(r.Context(), accessAdminOrgID(r), chi.URLParam(r, "id"), accessAdminActorID(r), input); err != nil {
		writeAccessAdministrationError(w, r, err, "Failed to assign role")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"message": "Role assigned"})
}

func (h *AccessAdministrationHandler) UnassignRole(w http.ResponseWriter, r *http.Request) {
	var input models.ManagedRoleUnassignmentInput
	if err := decodeAuditJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	if err := h.service.UnassignRole(r.Context(), accessAdminOrgID(r), chi.URLParam(r, "id"), chi.URLParam(r, "userID"), accessAdminActorID(r), input); err != nil {
		writeAccessAdministrationError(w, r, err, "Failed to remove role assignment")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AccessAdministrationHandler) ListEvents(w http.ResponseWriter, r *http.Request) {
	pagination := parsePagination(r)
	items, total, err := h.service.ListEvents(r.Context(), accessAdminOrgID(r), chi.URLParam(r, "id"), pagination)
	if err != nil {
		writeAccessAdministrationError(w, r, err, "Failed to list role history")
		return
	}
	writePaginated(w, items, total, normalizedHandlerPagination(pagination))
}

func accessAdminOrgID(r *http.Request) string   { return middleware.GetOrgIDFromContext(r.Context()) }
func accessAdminActorID(r *http.Request) string { return middleware.GetUserIDFromContext(r.Context()) }

func writeAccessAdministrationError(w http.ResponseWriter, r *http.Request, err error, fallback string) {
	switch {
	case errors.Is(err, service.ErrManagedRoleInvalid):
		writeError(w, http.StatusBadRequest, "Invalid role request", err.Error())
	case errors.Is(err, service.ErrManagedRoleNotFound):
		writeError(w, http.StatusNotFound, "Role not found", "")
	case errors.Is(err, service.ErrManagedRoleConflict):
		writeError(w, http.StatusConflict, "Role conflicts with current state", err.Error())
	case errors.Is(err, service.ErrManagedRoleImmutable):
		writeError(w, http.StatusConflict, "System roles cannot be changed", "Clone the role to create a tenant-owned variant")
	case errors.Is(err, service.ErrManagedRoleInUse):
		writeError(w, http.StatusConflict, "Role is assigned to users", "Remove or transfer assignments before deleting the role")
	case errors.Is(err, service.ErrLastTenantAdministrator):
		writeError(w, http.StatusConflict, "The organization must retain an administrator", "Assign another administrative role before removing this grant")
	case errors.Is(err, service.ErrManagedRolePermission), errors.Is(err, service.ErrRoleAssignmentInvalid):
		writeError(w, http.StatusUnprocessableEntity, "Role assignment or permission is not valid", err.Error())
	default:
		log.Error().Err(err).
			Str("request_id", middleware.GetRequestIDFromContext(r.Context())).
			Str("organization_id", accessAdminOrgID(r)).Msg("access administration request failed")
		writeError(w, http.StatusInternalServerError, fallback, "")
	}
}

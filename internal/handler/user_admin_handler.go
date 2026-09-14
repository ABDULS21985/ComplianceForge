package handler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"github.com/complianceforge/platform/internal/middleware"
	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/service"
)

type UserAdministrationService interface {
	CreateUser(context.Context, string, string, models.DirectoryUserCreateInput) (*models.DirectoryUser, error)
	GetUser(context.Context, string, string) (*models.DirectoryUser, error)
	ListUsers(context.Context, string, models.DirectoryUserListFilter) ([]models.DirectoryUser, int, error)
	UpdateUser(context.Context, string, string, string, models.DirectoryUserPatch) (*models.DirectoryUser, error)
	SuspendUser(context.Context, string, string, string, models.DirectoryUserStateInput) (*models.DirectoryUser, error)
	ReactivateUser(context.Context, string, string, string, models.DirectoryUserStateInput) (*models.DirectoryUser, error)
	PreviewOwnership(context.Context, string, string) (*models.DirectoryOwnershipImpact, error)
	TransferOwnership(context.Context, string, string, string, models.DirectoryOwnershipTransferInput) (*models.DirectoryOwnershipImpact, *models.DirectoryUser, error)
	DeprovisionUser(context.Context, string, string, string, models.DirectoryUserDeprovisionInput) error
	ListUserEvents(context.Context, string, string, models.PaginationRequest) ([]models.DirectoryChangeEvent, int, error)
	CreateGroup(context.Context, string, string, models.DirectoryGroupCreateInput) (*models.DirectoryGroup, error)
	GetGroup(context.Context, string, string) (*models.DirectoryGroup, error)
	ListGroups(context.Context, string, models.DirectoryGroupListFilter) ([]models.DirectoryGroup, int, error)
	UpdateGroup(context.Context, string, string, string, models.DirectoryGroupPatch) (*models.DirectoryGroup, error)
	DeleteGroup(context.Context, string, string, string, int64, string) error
	ListGroupMembers(context.Context, string, string, models.PaginationRequest) ([]models.DirectoryUser, int, error)
	AddGroupMember(context.Context, string, string, string, models.DirectoryGroupMemberInput) (*models.DirectoryGroup, error)
	RemoveGroupMember(context.Context, string, string, string, string, models.DirectoryGroupMemberRemoveInput) (*models.DirectoryGroup, error)
	ChangeGroupMembers(context.Context, string, string, string, models.DirectoryGroupBulkMembersInput) (*models.DirectoryGroup, error)
	ListGroupEvents(context.Context, string, string, models.PaginationRequest) ([]models.DirectoryChangeEvent, int, error)
	PreviewImport(context.Context, string, []byte) (*models.DirectoryImportPreview, error)
	ApplyImport(context.Context, string, string, string, string, []byte) (*models.DirectoryImportResult, error)
}

type UserAdministrationHandler struct{ service UserAdministrationService }

func NewUserAdministrationHandler(service UserAdministrationService) *UserAdministrationHandler {
	return &UserAdministrationHandler{service: service}
}

func (h *UserAdministrationHandler) Ready() bool { return h != nil && h.service != nil }

func (h *UserAdministrationHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var input models.DirectoryUserCreateInput
	if err := decodeAuditJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := h.service.CreateUser(r.Context(), directoryOrgID(r), directoryActorID(r), input)
	if err != nil {
		writeUserAdministrationError(w, r, err, "Failed to create directory user")
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (h *UserAdministrationHandler) GetUser(w http.ResponseWriter, r *http.Request) {
	item, err := h.service.GetUser(r.Context(), directoryOrgID(r), chi.URLParam(r, "id"))
	if err != nil {
		writeUserAdministrationError(w, r, err, "Failed to get directory user")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *UserAdministrationHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	filter := models.DirectoryUserListFilter{PaginationRequest: parsePagination(r), Search: r.URL.Query().Get("search"),
		Status: r.URL.Query().Get("status"), Department: r.URL.Query().Get("department"), Location: r.URL.Query().Get("location"),
		ManagerID: r.URL.Query().Get("manager_user_id"), RoleSlug: r.URL.Query().Get("role_slug"), GroupID: r.URL.Query().Get("group_id"),
		SortBy: r.URL.Query().Get("sort_by"), SortDirection: r.URL.Query().Get("sort_dir")}
	items, total, err := h.service.ListUsers(r.Context(), directoryOrgID(r), filter)
	if err != nil {
		writeUserAdministrationError(w, r, err, "Failed to list directory users")
		return
	}
	writePaginated(w, items, total, normalizedHandlerPagination(filter.PaginationRequest))
}

func (h *UserAdministrationHandler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	var input models.DirectoryUserPatch
	if err := decodeAuditJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := h.service.UpdateUser(r.Context(), directoryOrgID(r), chi.URLParam(r, "id"), directoryActorID(r), input)
	if err != nil {
		writeUserAdministrationError(w, r, err, "Failed to update directory user")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *UserAdministrationHandler) SuspendUser(w http.ResponseWriter, r *http.Request) {
	h.changeUserState(w, r, h.service.SuspendUser, "Failed to suspend directory user")
}

func (h *UserAdministrationHandler) ReactivateUser(w http.ResponseWriter, r *http.Request) {
	h.changeUserState(w, r, h.service.ReactivateUser, "Failed to reactivate directory user")
}

type directoryStateChange func(context.Context, string, string, string, models.DirectoryUserStateInput) (*models.DirectoryUser, error)

func (h *UserAdministrationHandler) changeUserState(w http.ResponseWriter, r *http.Request, change directoryStateChange, fallback string) {
	var input models.DirectoryUserStateInput
	if err := decodeAuditJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := change(r.Context(), directoryOrgID(r), chi.URLParam(r, "id"), directoryActorID(r), input)
	if err != nil {
		writeUserAdministrationError(w, r, err, fallback)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *UserAdministrationHandler) PreviewOwnership(w http.ResponseWriter, r *http.Request) {
	impact, err := h.service.PreviewOwnership(r.Context(), directoryOrgID(r), chi.URLParam(r, "id"))
	if err != nil {
		writeUserAdministrationError(w, r, err, "Failed to preview user ownership impact")
		return
	}
	writeJSON(w, http.StatusOK, impact)
}

func (h *UserAdministrationHandler) TransferOwnership(w http.ResponseWriter, r *http.Request) {
	var input models.DirectoryOwnershipTransferInput
	if err := decodeAuditJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	impact, item, err := h.service.TransferOwnership(r.Context(), directoryOrgID(r), chi.URLParam(r, "id"), directoryActorID(r), input)
	if err != nil {
		writeUserAdministrationError(w, r, err, "Failed to transfer user ownership")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": item, "transferred": impact})
}

func (h *UserAdministrationHandler) DeprovisionUser(w http.ResponseWriter, r *http.Request) {
	var input models.DirectoryUserDeprovisionInput
	if err := decodeAuditJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	if err := h.service.DeprovisionUser(r.Context(), directoryOrgID(r), chi.URLParam(r, "id"), directoryActorID(r), input); err != nil {
		writeUserAdministrationError(w, r, err, "Failed to deprovision directory user")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *UserAdministrationHandler) ListUserEvents(w http.ResponseWriter, r *http.Request) {
	h.listEvents(w, r, h.service.ListUserEvents)
}

func (h *UserAdministrationHandler) CreateGroup(w http.ResponseWriter, r *http.Request) {
	var input models.DirectoryGroupCreateInput
	if err := decodeAuditJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := h.service.CreateGroup(r.Context(), directoryOrgID(r), directoryActorID(r), input)
	if err != nil {
		writeUserAdministrationError(w, r, err, "Failed to create directory group")
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (h *UserAdministrationHandler) GetGroup(w http.ResponseWriter, r *http.Request) {
	item, err := h.service.GetGroup(r.Context(), directoryOrgID(r), chi.URLParam(r, "groupID"))
	if err != nil {
		writeUserAdministrationError(w, r, err, "Failed to get directory group")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *UserAdministrationHandler) ListGroups(w http.ResponseWriter, r *http.Request) {
	filter := models.DirectoryGroupListFilter{PaginationRequest: parsePagination(r), Search: r.URL.Query().Get("search"),
		GroupType: r.URL.Query().Get("group_type"), SortBy: r.URL.Query().Get("sort_by"), SortDirection: r.URL.Query().Get("sort_dir")}
	items, total, err := h.service.ListGroups(r.Context(), directoryOrgID(r), filter)
	if err != nil {
		writeUserAdministrationError(w, r, err, "Failed to list directory groups")
		return
	}
	writePaginated(w, items, total, normalizedHandlerPagination(filter.PaginationRequest))
}

func (h *UserAdministrationHandler) UpdateGroup(w http.ResponseWriter, r *http.Request) {
	var input models.DirectoryGroupPatch
	if err := decodeAuditJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := h.service.UpdateGroup(r.Context(), directoryOrgID(r), chi.URLParam(r, "groupID"), directoryActorID(r), input)
	if err != nil {
		writeUserAdministrationError(w, r, err, "Failed to update directory group")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *UserAdministrationHandler) DeleteGroup(w http.ResponseWriter, r *http.Request) {
	version, reason, err := directoryDeleteInputs(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid group deletion", err.Error())
		return
	}
	if err := h.service.DeleteGroup(r.Context(), directoryOrgID(r), chi.URLParam(r, "groupID"), directoryActorID(r), version, reason); err != nil {
		writeUserAdministrationError(w, r, err, "Failed to delete directory group")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *UserAdministrationHandler) ListGroupMembers(w http.ResponseWriter, r *http.Request) {
	pagination := parsePagination(r)
	items, total, err := h.service.ListGroupMembers(r.Context(), directoryOrgID(r), chi.URLParam(r, "groupID"), pagination)
	if err != nil {
		writeUserAdministrationError(w, r, err, "Failed to list directory group members")
		return
	}
	writePaginated(w, items, total, normalizedHandlerPagination(pagination))
}

func (h *UserAdministrationHandler) AddGroupMember(w http.ResponseWriter, r *http.Request) {
	var input models.DirectoryGroupMemberInput
	if err := decodeAuditJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := h.service.AddGroupMember(r.Context(), directoryOrgID(r), chi.URLParam(r, "groupID"), directoryActorID(r), input)
	if err != nil {
		writeUserAdministrationError(w, r, err, "Failed to add directory group member")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *UserAdministrationHandler) RemoveGroupMember(w http.ResponseWriter, r *http.Request) {
	var input models.DirectoryGroupMemberRemoveInput
	if err := decodeAuditJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := h.service.RemoveGroupMember(r.Context(), directoryOrgID(r), chi.URLParam(r, "groupID"), chi.URLParam(r, "userID"), directoryActorID(r), input)
	if err != nil {
		writeUserAdministrationError(w, r, err, "Failed to remove directory group member")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *UserAdministrationHandler) ChangeGroupMembers(w http.ResponseWriter, r *http.Request) {
	var input models.DirectoryGroupBulkMembersInput
	if err := decodeAuditJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := h.service.ChangeGroupMembers(r.Context(), directoryOrgID(r), chi.URLParam(r, "groupID"), directoryActorID(r), input)
	if err != nil {
		writeUserAdministrationError(w, r, err, "Failed to update directory group members")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *UserAdministrationHandler) ListGroupEvents(w http.ResponseWriter, r *http.Request) {
	h.listEvents(w, r, h.service.ListGroupEvents)
}

type directoryEventLister func(context.Context, string, string, models.PaginationRequest) ([]models.DirectoryChangeEvent, int, error)

func (h *UserAdministrationHandler) listEvents(w http.ResponseWriter, r *http.Request, list directoryEventLister) {
	pagination := parsePagination(r)
	id := chi.URLParam(r, "id")
	if id == "" {
		id = chi.URLParam(r, "groupID")
	}
	items, total, err := list(r.Context(), directoryOrgID(r), id, pagination)
	if err != nil {
		writeUserAdministrationError(w, r, err, "Failed to list directory history")
		return
	}
	writePaginated(w, items, total, normalizedHandlerPagination(pagination))
}

func (h *UserAdministrationHandler) PreviewImport(w http.ResponseWriter, r *http.Request) {
	content, err := readDirectoryCSV(w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid CSV import", err.Error())
		return
	}
	preview, err := h.service.PreviewImport(r.Context(), directoryOrgID(r), content)
	if err != nil {
		writeUserAdministrationError(w, r, err, "Failed to preview directory import")
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

func (h *UserAdministrationHandler) ApplyImport(w http.ResponseWriter, r *http.Request) {
	content, err := readDirectoryCSV(w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid CSV import", err.Error())
		return
	}
	result, err := h.service.ApplyImport(r.Context(), directoryOrgID(r), directoryActorID(r), r.Header.Get("Idempotency-Key"), r.Header.Get("X-Change-Reason"), content)
	if err != nil {
		writeUserAdministrationError(w, r, err, "Failed to apply directory import")
		return
	}
	status := http.StatusCreated
	if result.Replayed {
		status = http.StatusOK
	}
	writeJSON(w, status, result)
}

func readDirectoryCSV(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	const maximum = int64(2<<20) + 1
	r.Body = http.MaxBytesReader(w, r.Body, maximum)
	content, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("read CSV: %w", err)
	}
	if len(content) == 0 || int64(len(content)) >= maximum {
		return nil, errors.New("CSV must contain 1 byte to 2 MiB")
	}
	return content, nil
}

func directoryDeleteInputs(r *http.Request) (int64, string, error) {
	value, err := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("expected_version")), 10, 64)
	if err != nil || value < 1 {
		return 0, "", errors.New("expected_version must be a positive integer")
	}
	reason := strings.TrimSpace(r.Header.Get("X-Change-Reason"))
	if reason == "" {
		return 0, "", errors.New("X-Change-Reason is required")
	}
	return value, reason, nil
}

func directoryOrgID(r *http.Request) string   { return middleware.GetOrgIDFromContext(r.Context()) }
func directoryActorID(r *http.Request) string { return middleware.GetUserIDFromContext(r.Context()) }

func writeUserAdministrationError(w http.ResponseWriter, r *http.Request, err error, fallback string) {
	switch {
	case errors.Is(err, service.ErrUserAdministrationInvalid):
		writeError(w, http.StatusBadRequest, "Invalid user administration request", err.Error())
	case errors.Is(err, service.ErrUserAdministrationNotFound):
		writeError(w, http.StatusNotFound, "Directory record not found", "")
	case errors.Is(err, service.ErrUserAdministrationVersion):
		writeError(w, http.StatusConflict, "Directory record has changed", "Refresh the record and retry with its current version.")
	case errors.Is(err, service.ErrUserAdministrationLastAdmin):
		writeError(w, http.StatusConflict, "Last active administrator is protected", "Assign another active administrator before continuing.")
	case errors.Is(err, service.ErrUserAdministrationOwnership):
		writeError(w, http.StatusConflict, "User still owns resources", "Preview impact and supply an active replacement user.")
	case errors.Is(err, service.ErrUserAdministrationDynamicGroup):
		writeError(w, http.StatusConflict, "Dynamic membership is rule-managed", "Update the dynamic membership rule instead.")
	case errors.Is(err, service.ErrUserAdministrationImportInvalid):
		writeError(w, http.StatusUnprocessableEntity, "CSV import contains invalid rows", "Run the preview endpoint and resolve every row error.")
	case errors.Is(err, service.ErrUserAdministrationIdempotency):
		writeError(w, http.StatusConflict, "Idempotency key conflict", "Use the original CSV or a new idempotency key.")
	case errors.Is(err, service.ErrSubscriptionLimitExceeded):
		writeError(w, http.StatusPaymentRequired, "User capacity reached", "Upgrade the subscription or deprovision an unused user before adding another seat.")
	case errors.Is(err, service.ErrUserAdministrationConflict):
		writeError(w, http.StatusConflict, "User administration conflict", err.Error())
	default:
		log.Error().Err(err).Str("request_id", middleware.GetRequestIDFromContext(r.Context())).
			Str("organization_id", directoryOrgID(r)).Msg("user administration request failed")
		writeError(w, http.StatusInternalServerError, fallback, "")
	}
}

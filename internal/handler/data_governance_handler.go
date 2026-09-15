package handler

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"github.com/complianceforge/platform/internal/middleware"
	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/service"
)

type DataGovernanceService interface {
	GetPolicy(context.Context, string) (*models.DataGovernancePolicy, error)
	UpsertPolicy(context.Context, string, string, string, models.DataGovernancePolicyInput) (*models.DataGovernancePolicy, error)
	CreateSchedule(context.Context, string, string, string, models.RetentionScheduleInput) (*models.RetentionSchedule, error)
	GetSchedule(context.Context, string, string) (*models.RetentionSchedule, error)
	ListSchedules(context.Context, string, models.RetentionScheduleFilter) ([]models.RetentionSchedule, int, error)
	UpdateSchedule(context.Context, string, string, string, string, models.RetentionSchedulePatch) (*models.RetentionSchedule, error)
	RetireSchedule(context.Context, string, string, string, string, int64, string) error
	CreateAssignment(context.Context, string, string, string, models.RetentionAssignmentInput) (*models.RecordRetentionAssignment, error)
	GetAssignment(context.Context, string, string) (*models.RecordRetentionAssignment, error)
	GetRecordDisposition(context.Context, string, string, string) (*models.RecordDispositionDecision, error)
	ReviewDisposition(context.Context, string, string, string, string, models.RetentionReviewInput) (*models.RecordRetentionAssignment, error)
	RequestException(context.Context, string, string, string, string, models.RetentionExceptionInput) (*models.RetentionException, error)
	DecideException(context.Context, string, string, string, string, models.RetentionExceptionDecisionInput) (*models.RetentionException, error)
	ListExceptions(context.Context, string, string) ([]models.RetentionException, error)
	CreateLegalHold(context.Context, string, string, string, models.LegalHoldInput) (*models.LegalHold, error)
	GetLegalHold(context.Context, string, string) (*models.LegalHold, error)
	ListLegalHolds(context.Context, string, string, models.PaginationRequest) ([]models.LegalHold, int, error)
	UpdateLegalHold(context.Context, string, string, string, string, models.LegalHoldPatch) (*models.LegalHold, error)
	ReleaseLegalHold(context.Context, string, string, string, string, models.LegalHoldReleaseInput) (*models.LegalHold, error)
	AddLegalHoldRecord(context.Context, string, string, string, string, models.LegalHoldRecordInput) (*models.LegalHoldRecord, error)
	ReleaseLegalHoldRecord(context.Context, string, string, string, string, string, models.LegalHoldRecordReleaseInput) error
	ListLegalHoldRecords(context.Context, string, string, bool) ([]models.LegalHoldRecord, error)
	ListEvents(context.Context, string, models.DataGovernanceEventFilter) ([]models.DataGovernanceEvent, int, error)
	VerifyEventChain(context.Context, string) (*models.GovernanceChainVerification, error)
}

type DataGovernanceHandler struct{ service DataGovernanceService }

func NewDataGovernanceHandler(service DataGovernanceService) *DataGovernanceHandler {
	return &DataGovernanceHandler{service: service}
}

func (h *DataGovernanceHandler) Ready() bool {
	if h == nil || h.service == nil {
		return false
	}
	value := reflect.ValueOf(h.service)
	return !((value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface ||
		value.Kind() == reflect.Map || value.Kind() == reflect.Slice || value.Kind() == reflect.Func) && value.IsNil())
}

func (h *DataGovernanceHandler) GetPolicy(w http.ResponseWriter, r *http.Request) {
	item, err := h.service.GetPolicy(r.Context(), governanceOrgID(r))
	if err != nil {
		writeDataGovernanceError(w, r, err, "Failed to load data governance policy")
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "settings", item)
}

func (h *DataGovernanceHandler) UpsertPolicy(w http.ResponseWriter, r *http.Request) {
	var input models.DataGovernancePolicyInput
	if err := decodeAuditJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := h.service.UpsertPolicy(
		r.Context(), governanceOrgID(r), governanceActorID(r), governanceRequestID(r), input,
	)
	if err != nil {
		writeDataGovernanceError(w, r, err, "Failed to save data governance policy")
		return
	}
	status := http.StatusOK
	if input.ExpectedVersion == nil {
		status = http.StatusCreated
	}
	writeClassifiedJSON(w, r, status, "settings", item)
}

func (h *DataGovernanceHandler) ListSchedules(w http.ResponseWriter, r *http.Request) {
	filter := models.RetentionScheduleFilter{
		PaginationRequest: parsePagination(r),
		RecordType:        r.URL.Query().Get("record_type"), Status: r.URL.Query().Get("status"),
		DataClassification: r.URL.Query().Get("classification"),
		Jurisdiction:       r.URL.Query().Get("jurisdiction"), Search: r.URL.Query().Get("search"),
		SortBy: r.URL.Query().Get("sort_by"), SortDirection: r.URL.Query().Get("sort_direction"),
	}
	items, total, err := h.service.ListSchedules(r.Context(), governanceOrgID(r), filter)
	if err != nil {
		writeDataGovernanceError(w, r, err, "Failed to list retention schedules")
		return
	}
	writeClassifiedPaginated(w, r, "settings", items, total, normalizedHandlerPagination(filter.PaginationRequest))
}

func (h *DataGovernanceHandler) CreateSchedule(w http.ResponseWriter, r *http.Request) {
	var input models.RetentionScheduleInput
	if err := decodeAuditJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := h.service.CreateSchedule(
		r.Context(), governanceOrgID(r), governanceActorID(r), governanceRequestID(r), input,
	)
	if err != nil {
		writeDataGovernanceError(w, r, err, "Failed to create retention schedule")
		return
	}
	writeClassifiedJSON(w, r, http.StatusCreated, "settings", item)
}

func (h *DataGovernanceHandler) GetSchedule(w http.ResponseWriter, r *http.Request) {
	item, err := h.service.GetSchedule(r.Context(), governanceOrgID(r), chi.URLParam(r, "scheduleID"))
	if err != nil {
		writeDataGovernanceError(w, r, err, "Failed to load retention schedule")
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "settings", item)
}

func (h *DataGovernanceHandler) UpdateSchedule(w http.ResponseWriter, r *http.Request) {
	var input models.RetentionSchedulePatch
	if err := decodeAuditJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := h.service.UpdateSchedule(
		r.Context(), governanceOrgID(r), chi.URLParam(r, "scheduleID"), governanceActorID(r),
		governanceRequestID(r), input,
	)
	if err != nil {
		writeDataGovernanceError(w, r, err, "Failed to update retention schedule")
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "settings", item)
}

func (h *DataGovernanceHandler) RetireSchedule(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ExpectedVersion int64  `json:"expected_version"`
		Reason          string `json:"reason"`
	}
	if err := decodeAuditJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	if err := h.service.RetireSchedule(
		r.Context(), governanceOrgID(r), chi.URLParam(r, "scheduleID"), governanceActorID(r),
		governanceRequestID(r), input.ExpectedVersion, input.Reason,
	); err != nil {
		writeDataGovernanceError(w, r, err, "Failed to retire retention schedule")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *DataGovernanceHandler) CreateAssignment(w http.ResponseWriter, r *http.Request) {
	var input models.RetentionAssignmentInput
	if err := decodeAuditJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := h.service.CreateAssignment(
		r.Context(), governanceOrgID(r), governanceActorID(r), governanceRequestID(r), input,
	)
	if err != nil {
		writeDataGovernanceError(w, r, err, "Failed to assign record retention")
		return
	}
	writeClassifiedJSON(w, r, http.StatusCreated, "settings", item)
}

func (h *DataGovernanceHandler) GetAssignment(w http.ResponseWriter, r *http.Request) {
	item, err := h.service.GetAssignment(r.Context(), governanceOrgID(r), chi.URLParam(r, "assignmentID"))
	if err != nil {
		writeDataGovernanceError(w, r, err, "Failed to load record retention")
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "settings", item)
}

func (h *DataGovernanceHandler) GetRecordDisposition(w http.ResponseWriter, r *http.Request) {
	item, err := h.service.GetRecordDisposition(
		r.Context(), governanceOrgID(r), chi.URLParam(r, "recordType"), chi.URLParam(r, "recordID"),
	)
	if err != nil {
		writeDataGovernanceError(w, r, err, "Failed to evaluate record disposition")
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "settings", item)
}

func (h *DataGovernanceHandler) ReviewDisposition(w http.ResponseWriter, r *http.Request) {
	var input models.RetentionReviewInput
	if err := decodeAuditJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := h.service.ReviewDisposition(
		r.Context(), governanceOrgID(r), chi.URLParam(r, "assignmentID"), governanceActorID(r),
		governanceRequestID(r), input,
	)
	if err != nil {
		writeDataGovernanceError(w, r, err, "Failed to review record disposition")
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "settings", item)
}

func (h *DataGovernanceHandler) ListExceptions(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.ListExceptions(r.Context(), governanceOrgID(r), chi.URLParam(r, "assignmentID"))
	if err != nil {
		writeDataGovernanceError(w, r, err, "Failed to list retention exceptions")
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "settings", map[string]any{"data": items})
}

func (h *DataGovernanceHandler) RequestException(w http.ResponseWriter, r *http.Request) {
	var input models.RetentionExceptionInput
	if err := decodeAuditJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := h.service.RequestException(
		r.Context(), governanceOrgID(r), chi.URLParam(r, "assignmentID"), governanceActorID(r),
		governanceRequestID(r), input,
	)
	if err != nil {
		writeDataGovernanceError(w, r, err, "Failed to request retention exception")
		return
	}
	writeClassifiedJSON(w, r, http.StatusCreated, "settings", item)
}

func (h *DataGovernanceHandler) DecideException(w http.ResponseWriter, r *http.Request) {
	var input models.RetentionExceptionDecisionInput
	if err := decodeAuditJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := h.service.DecideException(
		r.Context(), governanceOrgID(r), chi.URLParam(r, "exceptionID"), governanceActorID(r),
		governanceRequestID(r), input,
	)
	if err != nil {
		writeDataGovernanceError(w, r, err, "Failed to decide retention exception")
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "settings", item)
}

func (h *DataGovernanceHandler) ListLegalHolds(w http.ResponseWriter, r *http.Request) {
	pagination := parsePagination(r)
	items, total, err := h.service.ListLegalHolds(
		r.Context(), governanceOrgID(r), r.URL.Query().Get("status"), pagination,
	)
	if err != nil {
		writeDataGovernanceError(w, r, err, "Failed to list legal holds")
		return
	}
	writeClassifiedPaginated(w, r, "settings", items, total, normalizedHandlerPagination(pagination))
}

func (h *DataGovernanceHandler) CreateLegalHold(w http.ResponseWriter, r *http.Request) {
	var input models.LegalHoldInput
	if err := decodeAuditJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := h.service.CreateLegalHold(
		r.Context(), governanceOrgID(r), governanceActorID(r), governanceRequestID(r), input,
	)
	if err != nil {
		writeDataGovernanceError(w, r, err, "Failed to create legal hold")
		return
	}
	writeClassifiedJSON(w, r, http.StatusCreated, "settings", item)
}

func (h *DataGovernanceHandler) GetLegalHold(w http.ResponseWriter, r *http.Request) {
	item, err := h.service.GetLegalHold(r.Context(), governanceOrgID(r), chi.URLParam(r, "holdID"))
	if err != nil {
		writeDataGovernanceError(w, r, err, "Failed to load legal hold")
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "settings", item)
}

func (h *DataGovernanceHandler) UpdateLegalHold(w http.ResponseWriter, r *http.Request) {
	var input models.LegalHoldPatch
	if err := decodeAuditJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := h.service.UpdateLegalHold(
		r.Context(), governanceOrgID(r), chi.URLParam(r, "holdID"), governanceActorID(r),
		governanceRequestID(r), input,
	)
	if err != nil {
		writeDataGovernanceError(w, r, err, "Failed to update legal hold")
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "settings", item)
}

func (h *DataGovernanceHandler) ReleaseLegalHold(w http.ResponseWriter, r *http.Request) {
	var input models.LegalHoldReleaseInput
	if err := decodeAuditJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := h.service.ReleaseLegalHold(
		r.Context(), governanceOrgID(r), chi.URLParam(r, "holdID"), governanceActorID(r),
		governanceRequestID(r), input,
	)
	if err != nil {
		writeDataGovernanceError(w, r, err, "Failed to close legal hold")
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "settings", item)
}

func (h *DataGovernanceHandler) ListLegalHoldRecords(w http.ResponseWriter, r *http.Request) {
	activeOnly := true
	if raw := strings.TrimSpace(r.URL.Query().Get("active_only")); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Invalid active_only value", "active_only must be true or false")
			return
		}
		activeOnly = parsed
	}
	items, err := h.service.ListLegalHoldRecords(
		r.Context(), governanceOrgID(r), chi.URLParam(r, "holdID"), activeOnly,
	)
	if err != nil {
		writeDataGovernanceError(w, r, err, "Failed to list legal hold records")
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "settings", map[string]any{"data": items})
}

func (h *DataGovernanceHandler) AddLegalHoldRecord(w http.ResponseWriter, r *http.Request) {
	var input models.LegalHoldRecordInput
	if err := decodeAuditJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := h.service.AddLegalHoldRecord(
		r.Context(), governanceOrgID(r), chi.URLParam(r, "holdID"), governanceActorID(r),
		governanceRequestID(r), input,
	)
	if err != nil {
		writeDataGovernanceError(w, r, err, "Failed to add record to legal hold")
		return
	}
	writeClassifiedJSON(w, r, http.StatusCreated, "settings", item)
}

func (h *DataGovernanceHandler) ReleaseLegalHoldRecord(w http.ResponseWriter, r *http.Request) {
	var input models.LegalHoldRecordReleaseInput
	if err := decodeAuditJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	if err := h.service.ReleaseLegalHoldRecord(
		r.Context(), governanceOrgID(r), chi.URLParam(r, "holdID"), chi.URLParam(r, "holdRecordID"),
		governanceActorID(r), governanceRequestID(r), input,
	); err != nil {
		writeDataGovernanceError(w, r, err, "Failed to release record from legal hold")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *DataGovernanceHandler) ListEvents(w http.ResponseWriter, r *http.Request) {
	filter := models.DataGovernanceEventFilter{
		PaginationRequest: parsePagination(r), EntityType: r.URL.Query().Get("entity_type"),
		EntityID: r.URL.Query().Get("entity_id"), EventType: r.URL.Query().Get("event_type"),
	}
	items, total, err := h.service.ListEvents(r.Context(), governanceOrgID(r), filter)
	if err != nil {
		writeDataGovernanceError(w, r, err, "Failed to list data governance history")
		return
	}
	writeClassifiedPaginated(w, r, "settings", items, total, normalizedHandlerPagination(filter.PaginationRequest))
}

func (h *DataGovernanceHandler) VerifyEventChain(w http.ResponseWriter, r *http.Request) {
	item, err := h.service.VerifyEventChain(r.Context(), governanceOrgID(r))
	if err != nil {
		writeDataGovernanceError(w, r, err, "Failed to verify data governance history")
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "settings", item)
}

func governanceOrgID(r *http.Request) string { return middleware.GetOrgIDFromContext(r.Context()) }

func governanceActorID(r *http.Request) string { return middleware.GetUserIDFromContext(r.Context()) }

func governanceRequestID(r *http.Request) string {
	return middleware.GetRequestIDFromContext(r.Context())
}

func writeDataGovernanceError(w http.ResponseWriter, r *http.Request, err error, fallback string) {
	switch {
	case errors.Is(err, service.ErrDataGovernanceInvalid):
		writeError(w, http.StatusBadRequest, "Invalid data governance request", err.Error())
	case errors.Is(err, service.ErrDataGovernanceNotFound):
		writeError(w, http.StatusNotFound, "Data governance record not found", "")
	case errors.Is(err, service.ErrDataGovernanceConflict):
		writeError(w, http.StatusConflict, "Data governance record changed", "Reload the current record and retry with its version")
	case errors.Is(err, service.ErrDataGovernanceState):
		writeError(w, http.StatusConflict, "Data governance operation is not allowed", err.Error())
	default:
		log.Error().Err(err).
			Str("request_id", governanceRequestID(r)).
			Str("organization_id", governanceOrgID(r)).Msg("data governance request failed")
		writeError(w, http.StatusInternalServerError, fallback, "")
	}
}

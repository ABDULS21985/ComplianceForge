package handler

import (
	"context"
	"encoding/json"
	"errors"
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

type IncidentService interface {
	Create(context.Context, string, string, models.IncidentCreateInput) (*models.Incident, error)
	GetByID(context.Context, string, string) (*models.Incident, error)
	Update(context.Context, string, string, string, models.IncidentPatch) (*models.Incident, error)
	Delete(context.Context, string, string, string, int64) error
	List(context.Context, string, models.IncidentListFilter) ([]models.Incident, int, error)
	Transition(context.Context, string, string, string, models.IncidentTransitionInput) (*models.Incident, error)
	Cancel(context.Context, string, string, string, int64, string) (*models.Incident, error)
	Close(context.Context, string, string, string, int64, string) (*models.Incident, error)
	Reopen(context.Context, string, string, string, int64, string) (*models.Incident, error)
	Escalate(context.Context, string, string, string, models.IncidentEscalationInput) (*models.Incident, error)
	AssessBreach(context.Context, string, string, string, models.IncidentBreachAssessmentInput) (*models.Incident, error)
	NotifyDPA(context.Context, string, string, string, models.IncidentDPANotificationInput) (*models.Incident, error)
	ListBreachDue(context.Context, string, int, int) ([]models.Incident, error)
	CreateAssignment(context.Context, string, string, string, models.IncidentAssignmentInput) (*models.IncidentAssignment, *models.Incident, error)
	Unassign(context.Context, string, string, string, string, models.IncidentUnassignmentInput) (*models.IncidentAssignment, *models.Incident, error)
	ListAssignments(context.Context, string, string, bool) ([]models.IncidentAssignment, error)
	ListEvents(context.Context, string, string, models.PaginationRequest) ([]models.IncidentEvent, int, error)
	Statistics(context.Context, string) (*models.IncidentStatistics, error)
}

type IncidentHandler struct{ service IncidentService }

func NewIncidentHandler(service IncidentService) *IncidentHandler {
	return &IncidentHandler{service: service}
}
func (h *IncidentHandler) Ready() bool { return h != nil && h.service != nil }

func (h *IncidentHandler) Create(w http.ResponseWriter, r *http.Request) {
	var input models.IncidentCreateInput
	if err := decodeIncidentJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := h.service.Create(r.Context(), incidentOrgID(r), incidentUserID(r), input)
	if err != nil {
		writeIncidentError(w, r, err, "Failed to create incident")
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (h *IncidentHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	item, err := h.service.GetByID(r.Context(), incidentOrgID(r), chi.URLParam(r, "id"))
	if err != nil {
		writeIncidentError(w, r, err, "Failed to get incident")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *IncidentHandler) Update(w http.ResponseWriter, r *http.Request) {
	var patch models.IncidentPatch
	if err := decodeIncidentJSON(w, r, &patch); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := h.service.Update(r.Context(), incidentOrgID(r), incidentUserID(r), chi.URLParam(r, "id"), patch)
	if err != nil {
		writeIncidentError(w, r, err, "Failed to update incident")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *IncidentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	version, err := positiveQueryInteger(r, "version", 0, 0)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid version", err.Error())
		return
	}
	if err := h.service.Delete(r.Context(), incidentOrgID(r), incidentUserID(r), chi.URLParam(r, "id"), int64(version)); err != nil {
		writeIncidentError(w, r, err, "Failed to delete incident")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *IncidentHandler) List(w http.ResponseWriter, r *http.Request) {
	filter := models.IncidentListFilter{
		PaginationRequest: parsePagination(r),
		Status:            r.URL.Query().Get("status"), Severity: r.URL.Query().Get("severity"),
		Category: r.URL.Query().Get("category"), AssigneeID: r.URL.Query().Get("assignee_id"),
		Search: r.URL.Query().Get("search"), Sort: r.URL.Query().Get("sort"),
		Direction: r.URL.Query().Get("direction"),
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("breach_notifiable")); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Invalid breach_notifiable filter", "must be true or false")
			return
		}
		filter.Breach = &value
	}
	items, total, err := h.service.List(r.Context(), incidentOrgID(r), filter)
	if err != nil {
		writeIncidentError(w, r, err, "Failed to list incidents")
		return
	}
	pagination := normalizedHandlerPagination(filter.PaginationRequest)
	writePaginated(w, items, total, pagination)
}

func (h *IncidentHandler) Transition(w http.ResponseWriter, r *http.Request) {
	var input models.IncidentTransitionInput
	if err := decodeIncidentJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	if models.IncidentStatus(strings.ToLower(strings.TrimSpace(string(input.Status)))) == models.IncidentStatusClosed {
		writeError(w, http.StatusConflict, "Use the approved close action", "close the incident through /incidents/{id}/close")
		return
	}
	item, err := h.service.Transition(r.Context(), incidentOrgID(r), incidentUserID(r), chi.URLParam(r, "id"), input)
	if err != nil {
		writeIncidentError(w, r, err, "Failed to change incident lifecycle")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

// UpdateStatus retains the original endpoint while using the same validated
// lifecycle state machine as the explicit transition route.
func (h *IncidentHandler) UpdateStatus(w http.ResponseWriter, r *http.Request) { h.Transition(w, r) }

type incidentReasonInput struct {
	Version int64  `json:"version"`
	Reason  string `json:"reason"`
}

func (h *IncidentHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	h.changeWithReason(w, r, h.service.Cancel)
}

func (h *IncidentHandler) Close(w http.ResponseWriter, r *http.Request) {
	h.changeWithReason(w, r, h.service.Close)
}

func (h *IncidentHandler) Reopen(w http.ResponseWriter, r *http.Request) {
	h.changeWithReason(w, r, h.service.Reopen)
}

func (h *IncidentHandler) changeWithReason(w http.ResponseWriter, r *http.Request, operation func(context.Context, string, string, string, int64, string) (*models.Incident, error)) {
	var input incidentReasonInput
	if err := decodeIncidentJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := operation(r.Context(), incidentOrgID(r), incidentUserID(r), chi.URLParam(r, "id"), input.Version, input.Reason)
	if err != nil {
		writeIncidentError(w, r, err, "Failed to change incident lifecycle")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *IncidentHandler) Escalate(w http.ResponseWriter, r *http.Request) {
	var input models.IncidentEscalationInput
	if err := decodeIncidentJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := h.service.Escalate(r.Context(), incidentOrgID(r), incidentUserID(r), chi.URLParam(r, "id"), input)
	if err != nil {
		writeIncidentError(w, r, err, "Failed to escalate incident")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *IncidentHandler) AssessBreach(w http.ResponseWriter, r *http.Request) {
	var input models.IncidentBreachAssessmentInput
	if err := decodeIncidentJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := h.service.AssessBreach(r.Context(), incidentOrgID(r), incidentUserID(r), chi.URLParam(r, "id"), input)
	if err != nil {
		writeIncidentError(w, r, err, "Failed to record breach assessment")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *IncidentHandler) NotifyDPA(w http.ResponseWriter, r *http.Request) {
	var input models.IncidentDPANotificationInput
	if err := decodeIncidentJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := h.service.NotifyDPA(r.Context(), incidentOrgID(r), incidentUserID(r), chi.URLParam(r, "id"), input)
	if err != nil {
		writeIncidentError(w, r, err, "Failed to record supervisory authority notification")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *IncidentHandler) GetBreachNotifiable(w http.ResponseWriter, r *http.Request) {
	horizon, err := positiveQueryInteger(r, "horizon_hours", 168, 720)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid horizon_hours", err.Error())
		return
	}
	limit, err := positiveQueryInteger(r, "limit", 100, 200)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid limit", err.Error())
		return
	}
	items, err := h.service.ListBreachDue(r.Context(), incidentOrgID(r), horizon, limit)
	if err != nil {
		writeIncidentError(w, r, err, "Failed to list breach deadlines")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (h *IncidentHandler) Statistics(w http.ResponseWriter, r *http.Request) {
	stats, err := h.service.Statistics(r.Context(), incidentOrgID(r))
	if err != nil {
		writeIncidentError(w, r, err, "Failed to calculate incident statistics")
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (h *IncidentHandler) CreateAssignment(w http.ResponseWriter, r *http.Request) {
	var input models.IncidentAssignmentInput
	if err := decodeIncidentJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	assignment, incident, err := h.service.CreateAssignment(r.Context(), incidentOrgID(r), incidentUserID(r), chi.URLParam(r, "id"), input)
	if err != nil {
		writeIncidentError(w, r, err, "Failed to assign incident responder")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"assignment": assignment, "incident": incident})
}

func (h *IncidentHandler) Unassign(w http.ResponseWriter, r *http.Request) {
	var input models.IncidentUnassignmentInput
	if err := decodeIncidentJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	assignment, incident, err := h.service.Unassign(r.Context(), incidentOrgID(r), incidentUserID(r), chi.URLParam(r, "id"), chi.URLParam(r, "assignmentID"), input)
	if err != nil {
		writeIncidentError(w, r, err, "Failed to unassign incident responder")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"assignment": assignment, "incident": incident})
}

func (h *IncidentHandler) ListAssignments(w http.ResponseWriter, r *http.Request) {
	activeOnly := true
	if raw := strings.TrimSpace(r.URL.Query().Get("active_only")); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Invalid active_only filter", "must be true or false")
			return
		}
		activeOnly = value
	}
	items, err := h.service.ListAssignments(r.Context(), incidentOrgID(r), chi.URLParam(r, "id"), activeOnly)
	if err != nil {
		writeIncidentError(w, r, err, "Failed to list incident assignments")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (h *IncidentHandler) ListEvents(w http.ResponseWriter, r *http.Request) {
	pagination := normalizedHandlerPagination(parsePagination(r))
	items, total, err := h.service.ListEvents(r.Context(), incidentOrgID(r), chi.URLParam(r, "id"), pagination)
	if err != nil {
		writeIncidentError(w, r, err, "Failed to list incident timeline")
		return
	}
	writePaginated(w, items, total, pagination)
}

func incidentOrgID(r *http.Request) string  { return middleware.GetOrgIDFromContext(r.Context()) }
func incidentUserID(r *http.Request) string { return middleware.GetUserIDFromContext(r.Context()) }

func decodeIncidentJSON(w http.ResponseWriter, r *http.Request, destination any) error {
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

func positiveQueryInteger(r *http.Request, name string, defaultValue, maximum int) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		return defaultValue, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 || maximum > 0 && value > maximum {
		return 0, errors.New("must be a positive integer within the supported limit")
	}
	return value, nil
}

func writeIncidentError(w http.ResponseWriter, r *http.Request, err error, fallback string) {
	switch {
	case errors.Is(err, service.ErrIncidentInvalid), errors.Is(err, service.ErrIncidentInvalidID):
		writeError(w, http.StatusBadRequest, "Invalid incident request", err.Error())
	case errors.Is(err, service.ErrIncidentNotFound):
		writeError(w, http.StatusNotFound, "Incident not found", "")
	case errors.Is(err, service.ErrIncidentInvalidTransition), errors.Is(err, service.ErrIncidentConflict),
		errors.Is(err, service.ErrIncidentVersionConflict), errors.Is(err, service.ErrIncidentIdempotency):
		writeError(w, http.StatusConflict, "Incident request conflicts with current state", err.Error())
	default:
		log.Error().Str("request_id", middleware.GetRequestIDFromContext(r.Context())).Str("error_class", "incident_internal").Msg("incident request failed")
		writeError(w, http.StatusInternalServerError, fallback, "")
	}
}

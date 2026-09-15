package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"github.com/complianceforge/platform/internal/middleware"
	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/service"
)

type AuditService interface {
	Create(context.Context, string, string, models.AuditCreateInput) (*models.Audit, error)
	GetByID(context.Context, string, string) (*models.Audit, error)
	Update(context.Context, string, string, models.AuditPatch) (*models.Audit, error)
	Delete(context.Context, string, string) error
	List(context.Context, string, models.AuditListFilter) ([]models.Audit, int, error)
	Start(context.Context, string, string) (*models.Audit, error)
	Complete(context.Context, string, string) (*models.Audit, error)
	Close(context.Context, string, string) (*models.Audit, error)
	Cancel(context.Context, string, string) (*models.Audit, error)
	CreateFinding(context.Context, string, string, string, models.AuditFindingInput) (*models.AuditFinding, error)
	GetFinding(context.Context, string, string, string) (*models.AuditFinding, error)
	UpdateFinding(context.Context, string, string, string, models.AuditFindingPatch) (*models.AuditFinding, error)
	DeleteFinding(context.Context, string, string, string) error
	ListFindings(context.Context, string, string, models.PaginationRequest) ([]models.AuditFinding, int, error)
	FindingStats(context.Context, string, string) (*models.AuditFindingStats, error)
}

type AuditHandler struct{ service AuditService }

func NewAuditHandler(service AuditService) *AuditHandler { return &AuditHandler{service: service} }
func (h *AuditHandler) Ready() bool                      { return h != nil && h.service != nil }

func (h *AuditHandler) Create(w http.ResponseWriter, r *http.Request) {
	var input models.AuditCreateInput
	if err := decodeAuditJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := h.service.Create(r.Context(), auditOrgID(r), auditUserID(r), input)
	if err != nil {
		writeAuditError(w, r, err, "Failed to create audit")
		return
	}
	writeClassifiedJSON(w, r, http.StatusCreated, "audits", item)
}

func (h *AuditHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	item, err := h.service.GetByID(r.Context(), auditOrgID(r), chi.URLParam(r, "id"))
	if err != nil {
		writeAuditError(w, r, err, "Failed to get audit")
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "audits", item)
}

func (h *AuditHandler) Update(w http.ResponseWriter, r *http.Request) {
	var patch models.AuditPatch
	if err := decodeAuditJSON(w, r, &patch); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := h.service.Update(r.Context(), auditOrgID(r), chi.URLParam(r, "id"), patch)
	if err != nil {
		writeAuditError(w, r, err, "Failed to update audit")
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "audits", item)
}

func (h *AuditHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.service.Delete(r.Context(), auditOrgID(r), chi.URLParam(r, "id")); err != nil {
		writeAuditError(w, r, err, "Failed to delete audit")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AuditHandler) List(w http.ResponseWriter, r *http.Request) {
	filter := models.AuditListFilter{
		PaginationRequest: parsePagination(r),
		Status:            r.URL.Query().Get("status"), AuditType: r.URL.Query().Get("audit_type"),
		LeadAuditorID: r.URL.Query().Get("lead_auditor_id"),
		FrameworkID:   r.URL.Query().Get("framework_id"), Search: r.URL.Query().Get("search"),
	}
	items, total, err := h.service.List(r.Context(), auditOrgID(r), filter)
	if err != nil {
		writeAuditError(w, r, err, "Failed to list audits")
		return
	}
	writeClassifiedPaginated(w, r, "audits", items, total, normalizedHandlerPagination(filter.PaginationRequest))
}

func (h *AuditHandler) Start(w http.ResponseWriter, r *http.Request) {
	item, err := h.service.Start(r.Context(), auditOrgID(r), chi.URLParam(r, "id"))
	h.writeLifecycleResult(w, r, item, err)
}

func (h *AuditHandler) Complete(w http.ResponseWriter, r *http.Request) {
	item, err := h.service.Complete(r.Context(), auditOrgID(r), chi.URLParam(r, "id"))
	h.writeLifecycleResult(w, r, item, err)
}

func (h *AuditHandler) Close(w http.ResponseWriter, r *http.Request) {
	item, err := h.service.Close(r.Context(), auditOrgID(r), chi.URLParam(r, "id"))
	h.writeLifecycleResult(w, r, item, err)
}

func (h *AuditHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	item, err := h.service.Cancel(r.Context(), auditOrgID(r), chi.URLParam(r, "id"))
	h.writeLifecycleResult(w, r, item, err)
}

func (h *AuditHandler) writeLifecycleResult(w http.ResponseWriter, r *http.Request, item *models.Audit, err error) {
	if err != nil {
		writeAuditError(w, r, err, "Failed to change audit lifecycle")
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "audits", item)
}

func (h *AuditHandler) CreateFinding(w http.ResponseWriter, r *http.Request) {
	var input models.AuditFindingInput
	if err := decodeAuditJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := h.service.CreateFinding(r.Context(), auditOrgID(r), chi.URLParam(r, "id"), auditUserID(r), input)
	if err != nil {
		writeAuditError(w, r, err, "Failed to create audit finding")
		return
	}
	writeClassifiedJSON(w, r, http.StatusCreated, "audits", item)
}

func (h *AuditHandler) ListFindings(w http.ResponseWriter, r *http.Request) {
	pagination := normalizedHandlerPagination(parsePagination(r))
	items, total, err := h.service.ListFindings(r.Context(), auditOrgID(r), chi.URLParam(r, "id"), pagination)
	if err != nil {
		writeAuditError(w, r, err, "Failed to list audit findings")
		return
	}
	writeClassifiedPaginated(w, r, "audits", items, total, pagination)
}

func (h *AuditHandler) GetFinding(w http.ResponseWriter, r *http.Request) {
	item, err := h.service.GetFinding(r.Context(), auditOrgID(r), chi.URLParam(r, "id"), chi.URLParam(r, "findingID"))
	if err != nil {
		writeAuditError(w, r, err, "Failed to get audit finding")
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "audits", item)
}

func (h *AuditHandler) UpdateFinding(w http.ResponseWriter, r *http.Request) {
	var patch models.AuditFindingPatch
	if err := decodeAuditJSON(w, r, &patch); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := h.service.UpdateFinding(r.Context(), auditOrgID(r), chi.URLParam(r, "id"), chi.URLParam(r, "findingID"), patch)
	if err != nil {
		writeAuditError(w, r, err, "Failed to update audit finding")
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "audits", item)
}

func (h *AuditHandler) DeleteFinding(w http.ResponseWriter, r *http.Request) {
	err := h.service.DeleteFinding(r.Context(), auditOrgID(r), chi.URLParam(r, "id"), chi.URLParam(r, "findingID"))
	if err != nil {
		writeAuditError(w, r, err, "Failed to delete audit finding")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AuditHandler) FindingStats(w http.ResponseWriter, r *http.Request) {
	item, err := h.service.FindingStats(r.Context(), auditOrgID(r), chi.URLParam(r, "id"))
	if err != nil {
		writeAuditError(w, r, err, "Failed to calculate finding statistics")
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "audits", item)
}

func auditOrgID(r *http.Request) string  { return middleware.GetOrgIDFromContext(r.Context()) }
func auditUserID(r *http.Request) string { return middleware.GetUserIDFromContext(r.Context()) }

func decodeAuditJSON(w http.ResponseWriter, r *http.Request, destination any) error {
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

func normalizedHandlerPagination(p models.PaginationRequest) models.PaginationRequest {
	if p.Page < 1 {
		p.Page = 1
	}
	if p.PageSize < 1 || p.PageSize > 100 {
		p.PageSize = 20
	}
	return p
}

func writeAuditError(w http.ResponseWriter, r *http.Request, err error, fallback string) {
	switch {
	case errors.Is(err, service.ErrAuditInvalid), errors.Is(err, service.ErrAuditInvalidID), errors.Is(err, service.ErrFindingInvalid):
		writeError(w, http.StatusBadRequest, "Invalid audit request", err.Error())
	case errors.Is(err, service.ErrAuditNotFound), errors.Is(err, service.ErrFindingNotFound):
		writeError(w, http.StatusNotFound, "Audit resource not found", "")
	case errors.Is(err, service.ErrAuditInvalidReference), errors.Is(err, service.ErrAuditInvalidTransition),
		errors.Is(err, service.ErrFindingInvalidTransition), errors.Is(err, service.ErrAuditConflict):
		writeError(w, http.StatusConflict, "Audit request conflicts with current state", err.Error())
	default:
		message := strings.TrimSpace(fallback)
		if message == "" {
			message = "Audit request failed"
		}
		log.Error().Err(err).Str("request_id", middleware.GetRequestIDFromContext(r.Context())).Msg("audit request failed")
		writeError(w, http.StatusInternalServerError, message, "")
	}
}

package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/complianceforge/platform/internal/middleware"
	"github.com/complianceforge/platform/internal/models"
)

type ControlService interface {
	ListControls(context.Context, string, string, models.PaginationRequest) ([]models.Control, int, error)
	GetControl(context.Context, string, string) (*models.Control, error)
	UpdateControlImplementation(context.Context, string, string, models.ControlImplementationPatch) (*models.ControlImplementation, error)
	AttachControlEvidence(context.Context, string, string, string, models.AttachControlEvidenceInput) (*models.ControlEvidence, error)
	ListControlEvidence(context.Context, string, string, models.PaginationRequest) ([]models.ControlEvidence, int, error)
}

type ControlHandler struct{ service ControlService }

func NewControlHandler(service ControlService) *ControlHandler {
	return &ControlHandler{service: service}
}
func (h *ControlHandler) Ready() bool { return h != nil && h.service != nil }

func (h *ControlHandler) List(w http.ResponseWriter, r *http.Request) {
	p := parsePagination(r)
	items, total, err := h.service.ListControls(r.Context(), middleware.GetOrgIDFromContext(r.Context()), r.URL.Query().Get("framework_id"), p)
	if err != nil {
		writeComplianceError(w, err, "Failed to list controls")
		return
	}
	writePaginated(w, items, total, p)
}

func (h *ControlHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	item, err := h.service.GetControl(r.Context(), middleware.GetOrgIDFromContext(r.Context()), chi.URLParam(r, "id"))
	if err != nil {
		writeComplianceError(w, err, "Failed to get control")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *ControlHandler) UpdateImplementation(w http.ResponseWriter, r *http.Request) {
	var patch models.ControlImplementationPatch
	if err := decodeComplianceJSON(w, r, &patch); err != nil {
		return
	}
	item, err := h.service.UpdateControlImplementation(r.Context(), middleware.GetOrgIDFromContext(r.Context()), chi.URLParam(r, "id"), patch)
	if err != nil {
		writeComplianceError(w, err, "Failed to update control implementation")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *ControlHandler) AttachEvidence(w http.ResponseWriter, r *http.Request) {
	var input models.AttachControlEvidenceInput
	if err := decodeComplianceJSON(w, r, &input); err != nil {
		return
	}
	item, err := h.service.AttachControlEvidence(r.Context(), middleware.GetOrgIDFromContext(r.Context()), middleware.GetUserIDFromContext(r.Context()), chi.URLParam(r, "id"), input)
	if err != nil {
		writeComplianceError(w, err, "Failed to attach control evidence")
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (h *ControlHandler) ListEvidence(w http.ResponseWriter, r *http.Request) {
	p := parsePagination(r)
	items, total, err := h.service.ListControlEvidence(r.Context(), middleware.GetOrgIDFromContext(r.Context()), chi.URLParam(r, "id"), p)
	if err != nil {
		writeComplianceError(w, err, "Failed to list control evidence")
		return
	}
	writePaginated(w, items, total, p)
}

func decodeComplianceJSON(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return err
	}
	return nil
}

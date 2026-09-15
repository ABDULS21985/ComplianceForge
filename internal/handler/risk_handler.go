package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/complianceforge/platform/internal/middleware"
	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/service"
)

type RiskService interface {
	Create(context.Context, string, models.RiskCreateInput) (*models.Risk, error)
	GetByID(context.Context, string, string) (*models.Risk, error)
	Update(context.Context, string, string, models.RiskPatch) (*models.Risk, error)
	Assign(context.Context, string, string, *string, *string) (*models.Risk, error)
	Delete(context.Context, string, string) error
	List(context.Context, string, models.RiskListFilter) ([]models.Risk, int, error)
	ListCategories(context.Context, string) ([]models.RiskCategory, error)
	GetRiskMatrix(context.Context, string, string) (*models.RiskMatrixView, error)
	GetRiskHeatmap(context.Context, string) ([]models.RiskHeatmapEntry, error)
	CreateAssessment(context.Context, string, string, string, models.RiskAssessmentInput) (*models.RiskAssessment, error)
	ListAssessments(context.Context, string, string, models.PaginationRequest) ([]models.RiskAssessment, int, error)
	CreateTreatment(context.Context, string, string, string, models.RiskTreatmentInput) (*models.RiskTreatment, error)
	GetTreatment(context.Context, string, string, string) (*models.RiskTreatment, error)
	ListTreatments(context.Context, string, string, models.PaginationRequest) ([]models.RiskTreatment, int, error)
	UpdateTreatment(context.Context, string, string, string, models.RiskTreatmentPatch) (*models.RiskTreatment, error)
	ListAppetite(context.Context, string) ([]models.RiskAppetiteStatement, error)
	UpsertAppetite(context.Context, string, string, string, models.RiskAppetiteInput) (*models.RiskAppetiteStatement, error)
	ApproveAppetite(context.Context, string, string, string, models.RiskAppetiteInput) (*models.RiskAppetiteStatement, error)
	CreateIndicator(context.Context, string, string, string, models.RiskIndicatorInput) (*models.RiskIndicator, error)
	ListIndicators(context.Context, string, string) ([]models.RiskIndicator, error)
	RecordIndicatorValue(context.Context, string, string, string, string, models.RiskIndicatorValueInput) (*models.RiskIndicatorValue, error)
	ListIndicatorValues(context.Context, string, string, string, models.PaginationRequest) ([]models.RiskIndicatorValue, int, error)
}

type RiskHandler struct{ service RiskService }

func NewRiskHandler(service RiskService) *RiskHandler { return &RiskHandler{service: service} }
func (h *RiskHandler) Ready() bool                    { return h != nil && h.service != nil }

func (h *RiskHandler) Create(w http.ResponseWriter, r *http.Request) {
	var input models.RiskCreateInput
	if err := decodeRiskJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := h.service.Create(r.Context(), riskOrgID(r), input)
	if err != nil {
		writeRiskError(w, err, "Failed to create risk")
		return
	}
	writeClassifiedJSON(w, r, http.StatusCreated, "risks", item)
}

func (h *RiskHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	item, err := h.service.GetByID(r.Context(), riskOrgID(r), chi.URLParam(r, "id"))
	if err != nil {
		writeRiskError(w, err, "Failed to get risk")
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "risks", item)
}

func (h *RiskHandler) Update(w http.ResponseWriter, r *http.Request) {
	var patch models.RiskPatch
	if err := decodeRiskJSON(w, r, &patch); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := h.service.Update(r.Context(), riskOrgID(r), chi.URLParam(r, "id"), patch)
	if err != nil {
		writeRiskError(w, err, "Failed to update risk")
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "risks", item)
}

func (h *RiskHandler) Assign(w http.ResponseWriter, r *http.Request) {
	var input struct {
		OwnerUserID    *string `json:"owner_user_id"`
		DelegateUserID *string `json:"delegate_user_id"`
	}
	if err := decodeRiskJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := h.service.Assign(r.Context(), riskOrgID(r), chi.URLParam(r, "id"), input.OwnerUserID, input.DelegateUserID)
	if err != nil {
		writeRiskError(w, err, "Failed to assign risk")
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "risks", item)
}

func (h *RiskHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.service.Delete(r.Context(), riskOrgID(r), chi.URLParam(r, "id")); err != nil {
		writeRiskError(w, err, "Failed to delete risk")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *RiskHandler) List(w http.ResponseWriter, r *http.Request) {
	filter := models.RiskListFilter{
		PaginationRequest: parsePagination(r),
		Status:            r.URL.Query().Get("status"),
		Level:             r.URL.Query().Get("level"),
		CategoryID:        r.URL.Query().Get("category_id"),
		OwnerUserID:       r.URL.Query().Get("owner_user_id"),
		Search:            r.URL.Query().Get("search"),
	}
	if raw := r.URL.Query().Get("emerging"); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Invalid emerging filter", err.Error())
			return
		}
		filter.Emerging = &value
	}
	items, total, err := h.service.List(r.Context(), riskOrgID(r), filter)
	if err != nil {
		writeRiskError(w, err, "Failed to list risks")
		return
	}
	writeRiskPaginated(w, r, items, total, filter.PaginationRequest)
}

func (h *RiskHandler) GetMatrix(w http.ResponseWriter, r *http.Request) {
	item, err := h.service.GetRiskMatrix(r.Context(), riskOrgID(r), r.URL.Query().Get("dimension"))
	if err != nil {
		writeRiskError(w, err, "Failed to get risk matrix")
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "risks", map[string]any{"data": item})
}

func (h *RiskHandler) ListCategories(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.ListCategories(r.Context(), riskOrgID(r))
	if err != nil {
		writeRiskError(w, err, "Failed to list risk categories")
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "risks", map[string]any{"data": items})
}

func (h *RiskHandler) GetHeatmap(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.GetRiskHeatmap(r.Context(), riskOrgID(r))
	if err != nil {
		writeRiskError(w, err, "Failed to get risk heatmap")
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "risks", map[string]any{"data": items})
}

func (h *RiskHandler) CreateAssessment(w http.ResponseWriter, r *http.Request) {
	var input models.RiskAssessmentInput
	if err := decodeRiskJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := h.service.CreateAssessment(r.Context(), riskOrgID(r), chi.URLParam(r, "id"), riskUserID(r), input)
	if err != nil {
		writeRiskError(w, err, "Failed to create risk assessment")
		return
	}
	writeClassifiedJSON(w, r, http.StatusCreated, "risks", item)
}

func (h *RiskHandler) ListAssessments(w http.ResponseWriter, r *http.Request) {
	p := parsePagination(r)
	items, total, err := h.service.ListAssessments(r.Context(), riskOrgID(r), chi.URLParam(r, "id"), p)
	if err != nil {
		writeRiskError(w, err, "Failed to list risk assessments")
		return
	}
	writeRiskPaginated(w, r, items, total, p)
}

func (h *RiskHandler) CreateTreatment(w http.ResponseWriter, r *http.Request) {
	var input models.RiskTreatmentInput
	if err := decodeRiskJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := h.service.CreateTreatment(r.Context(), riskOrgID(r), chi.URLParam(r, "id"), riskUserID(r), input)
	if err != nil {
		writeRiskError(w, err, "Failed to create risk treatment")
		return
	}
	writeClassifiedJSON(w, r, http.StatusCreated, "risks", item)
}

func (h *RiskHandler) ListTreatments(w http.ResponseWriter, r *http.Request) {
	p := parsePagination(r)
	items, total, err := h.service.ListTreatments(r.Context(), riskOrgID(r), chi.URLParam(r, "id"), p)
	if err != nil {
		writeRiskError(w, err, "Failed to list risk treatments")
		return
	}
	writeRiskPaginated(w, r, items, total, p)
}

func (h *RiskHandler) GetTreatment(w http.ResponseWriter, r *http.Request) {
	item, err := h.service.GetTreatment(r.Context(), riskOrgID(r), chi.URLParam(r, "id"), chi.URLParam(r, "treatmentID"))
	if err != nil {
		writeRiskError(w, err, "Failed to get risk treatment")
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "risks", item)
}

func (h *RiskHandler) UpdateTreatment(w http.ResponseWriter, r *http.Request) {
	var patch models.RiskTreatmentPatch
	if err := decodeRiskJSON(w, r, &patch); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := h.service.UpdateTreatment(r.Context(), riskOrgID(r), chi.URLParam(r, "id"), chi.URLParam(r, "treatmentID"), patch)
	if err != nil {
		writeRiskError(w, err, "Failed to update risk treatment")
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "risks", item)
}

func (h *RiskHandler) ListAppetite(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.ListAppetite(r.Context(), riskOrgID(r))
	if err != nil {
		writeRiskError(w, err, "Failed to list risk appetite")
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "risks", map[string]any{"data": items})
}

func (h *RiskHandler) UpsertAppetite(w http.ResponseWriter, r *http.Request) {
	var input models.RiskAppetiteInput
	if err := decodeRiskJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := h.service.UpsertAppetite(r.Context(), riskOrgID(r), chi.URLParam(r, "categoryID"), riskUserID(r), input)
	if err != nil {
		writeRiskError(w, err, "Failed to update risk appetite")
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "risks", item)
}

func (h *RiskHandler) ApproveAppetite(w http.ResponseWriter, r *http.Request) {
	var input models.RiskAppetiteInput
	if err := decodeRiskJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := h.service.ApproveAppetite(r.Context(), riskOrgID(r), chi.URLParam(r, "categoryID"), riskUserID(r), input)
	if err != nil {
		writeRiskError(w, err, "Failed to approve risk appetite")
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "risks", item)
}

func (h *RiskHandler) CreateIndicator(w http.ResponseWriter, r *http.Request) {
	var input models.RiskIndicatorInput
	if err := decodeRiskJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := h.service.CreateIndicator(r.Context(), riskOrgID(r), chi.URLParam(r, "id"), riskUserID(r), input)
	if err != nil {
		writeRiskError(w, err, "Failed to create risk indicator")
		return
	}
	writeClassifiedJSON(w, r, http.StatusCreated, "risks", item)
}

func (h *RiskHandler) ListIndicators(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.ListIndicators(r.Context(), riskOrgID(r), chi.URLParam(r, "id"))
	if err != nil {
		writeRiskError(w, err, "Failed to list risk indicators")
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "risks", map[string]any{"data": items})
}

func (h *RiskHandler) RecordIndicatorValue(w http.ResponseWriter, r *http.Request) {
	var input models.RiskIndicatorValueInput
	if err := decodeRiskJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := h.service.RecordIndicatorValue(r.Context(), riskOrgID(r), chi.URLParam(r, "id"), chi.URLParam(r, "indicatorID"), riskUserID(r), input)
	if err != nil {
		writeRiskError(w, err, "Failed to record risk indicator value")
		return
	}
	writeClassifiedJSON(w, r, http.StatusCreated, "risks", item)
}

func (h *RiskHandler) ListIndicatorValues(w http.ResponseWriter, r *http.Request) {
	p := parsePagination(r)
	items, total, err := h.service.ListIndicatorValues(r.Context(), riskOrgID(r), chi.URLParam(r, "id"), chi.URLParam(r, "indicatorID"), p)
	if err != nil {
		writeRiskError(w, err, "Failed to list risk indicator values")
		return
	}
	writeRiskPaginated(w, r, items, total, p)
}

func riskOrgID(r *http.Request) string  { return middleware.GetOrgIDFromContext(r.Context()) }
func riskUserID(r *http.Request) string { return middleware.GetUserIDFromContext(r.Context()) }

func decodeRiskJSON(w http.ResponseWriter, r *http.Request, destination any) error {
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

func writeRiskPaginated(w http.ResponseWriter, r *http.Request, data any, total int, p models.PaginationRequest) {
	if p.Page < 1 {
		p.Page = 1
	}
	if p.PageSize < 1 || p.PageSize > 100 {
		p.PageSize = 20
	}
	writeClassifiedPaginated(w, r, "risks", data, total, p)
}

func writeRiskError(w http.ResponseWriter, err error, fallback string) {
	switch {
	case errors.Is(err, service.ErrSubscriptionLimitExceeded):
		writeError(w, http.StatusPaymentRequired, "Subscription risk limit reached", "Upgrade the subscription or archive an existing risk before creating another")
	case errors.Is(err, service.ErrInvalidRisk), errors.Is(err, service.ErrInvalidRiskID),
		errors.Is(err, service.ErrInvalidAssessment), errors.Is(err, service.ErrInvalidTreatment),
		errors.Is(err, service.ErrInvalidRiskAppetite), errors.Is(err, service.ErrInvalidRiskIndicator):
		writeError(w, http.StatusBadRequest, "Invalid risk request", err.Error())
	case errors.Is(err, service.ErrRiskNotFound), errors.Is(err, service.ErrRiskTreatmentNotFound), errors.Is(err, service.ErrRiskIndicatorNotFound):
		writeError(w, http.StatusNotFound, "Risk resource not found", err.Error())
	case errors.Is(err, service.ErrInvalidRiskReference), errors.Is(err, service.ErrInvalidRiskTransition),
		errors.Is(err, service.ErrInvalidTreatmentState), errors.Is(err, service.ErrRiskConflict):
		writeError(w, http.StatusConflict, "Risk request conflicts with current state", err.Error())
	default:
		message := strings.TrimSpace(fallback)
		if message == "" {
			message = "Risk request failed"
		}
		writeError(w, http.StatusInternalServerError, message, fmt.Sprintf("%v", err))
	}
}

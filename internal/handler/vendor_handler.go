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

type VendorService interface {
	Create(context.Context, string, string, models.VendorCreateInput) (*models.Vendor, error)
	GetByID(context.Context, string, string) (*models.Vendor, error)
	Update(context.Context, string, string, string, models.VendorPatch) (*models.Vendor, error)
	Delete(context.Context, string, string, string, int64) error
	List(context.Context, string, models.VendorListFilter) ([]models.Vendor, int, error)
	Transition(context.Context, string, string, string, models.VendorTransitionInput) (*models.Vendor, error)
	RecordAssessment(context.Context, string, string, string, models.VendorAssessmentInput) (*models.Vendor, error)
	Statistics(context.Context, string) (*models.VendorStatistics, error)
	ListDueForAssessment(context.Context, string, int, int) ([]models.Vendor, error)
	ListDueContracts(context.Context, string, int, int) ([]models.VendorDueContract, error)
	ListExpiringCertifications(context.Context, string, int, int) ([]models.VendorExpiringCertification, error)
	ListEvents(context.Context, string, string, models.PaginationRequest) ([]models.VendorEvent, int, error)
	SaveContact(context.Context, string, string, string, string, models.VendorContactInput) (*models.VendorContact, *models.Vendor, error)
	DeleteContact(context.Context, string, string, string, string, int64) (*models.Vendor, error)
	SaveContract(context.Context, string, string, string, string, models.VendorContractInput) (*models.VendorContract, *models.Vendor, error)
	DeleteContract(context.Context, string, string, string, string, int64) (*models.Vendor, error)
	SaveCertification(context.Context, string, string, string, string, models.VendorCertificationInput) (*models.VendorCertification, *models.Vendor, error)
	DeleteCertification(context.Context, string, string, string, string, int64) (*models.Vendor, error)
	SaveSubprocessor(context.Context, string, string, string, string, models.VendorSubprocessorInput) (*models.VendorSubprocessor, *models.Vendor, error)
	DeleteSubprocessor(context.Context, string, string, string, string, int64) (*models.Vendor, error)
}

type VendorHandler struct{ service VendorService }

func NewVendorHandler(service VendorService) *VendorHandler { return &VendorHandler{service: service} }
func (h *VendorHandler) Ready() bool                        { return h != nil && h.service != nil }

func (h *VendorHandler) Create(w http.ResponseWriter, r *http.Request) {
	var input models.VendorCreateInput
	if err := decodeAuditJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := h.service.Create(r.Context(), vendorOrgID(r), vendorUserID(r), input)
	if err != nil {
		writeVendorError(w, r, err, "Failed to create vendor")
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (h *VendorHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	item, err := h.service.GetByID(r.Context(), vendorOrgID(r), chi.URLParam(r, "id"))
	if err != nil {
		writeVendorError(w, r, err, "Failed to get vendor")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *VendorHandler) Update(w http.ResponseWriter, r *http.Request) {
	var input models.VendorPatch
	if err := decodeAuditJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := h.service.Update(r.Context(), vendorOrgID(r), vendorUserID(r), chi.URLParam(r, "id"), input)
	if err != nil {
		writeVendorError(w, r, err, "Failed to update vendor")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *VendorHandler) Delete(w http.ResponseWriter, r *http.Request) {
	version, err := requiredVendorVersion(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid version", err.Error())
		return
	}
	if err := h.service.Delete(r.Context(), vendorOrgID(r), vendorUserID(r), chi.URLParam(r, "id"), version); err != nil {
		writeVendorError(w, r, err, "Failed to delete vendor")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *VendorHandler) List(w http.ResponseWriter, r *http.Request) {
	filter := models.VendorListFilter{PaginationRequest: parsePagination(r), Status: r.URL.Query().Get("status"),
		Criticality: r.URL.Query().Get("criticality"), VendorTier: r.URL.Query().Get("vendor_tier"), RiskTier: r.URL.Query().Get("risk_tier"),
		OwnerUserID: r.URL.Query().Get("owner_user_id"), CountryCode: r.URL.Query().Get("country_code"), Search: r.URL.Query().Get("search"),
		SortBy: r.URL.Query().Get("sort_by"), SortDirection: r.URL.Query().Get("sort_dir")}
	for key, target := range map[string]**bool{"data_processing": &filter.DataProcessing, "due_assessment": &filter.DueAssessment} {
		if raw := strings.TrimSpace(r.URL.Query().Get(key)); raw != "" {
			value, err := strconv.ParseBool(raw)
			if err != nil {
				writeError(w, http.StatusBadRequest, "Invalid boolean filter", key+" must be true or false")
				return
			}
			*target = &value
		}
	}
	items, total, err := h.service.List(r.Context(), vendorOrgID(r), filter)
	if err != nil {
		writeVendorError(w, r, err, "Failed to list vendors")
		return
	}
	writePaginated(w, items, total, normalizedHandlerPagination(filter.PaginationRequest))
}

func (h *VendorHandler) Statistics(w http.ResponseWriter, r *http.Request) {
	item, err := h.service.Statistics(r.Context(), vendorOrgID(r))
	if err != nil {
		writeVendorError(w, r, err, "Failed to calculate vendor statistics")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *VendorHandler) Transition(w http.ResponseWriter, r *http.Request) {
	var input models.VendorTransitionInput
	if err := decodeAuditJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := h.service.Transition(r.Context(), vendorOrgID(r), vendorUserID(r), chi.URLParam(r, "id"), input)
	if err != nil {
		writeVendorError(w, r, err, "Failed to change vendor lifecycle")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *VendorHandler) RecordAssessment(w http.ResponseWriter, r *http.Request) {
	var input models.VendorAssessmentInput
	if err := decodeAuditJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	item, err := h.service.RecordAssessment(r.Context(), vendorOrgID(r), vendorUserID(r), chi.URLParam(r, "id"), input)
	if err != nil {
		writeVendorError(w, r, err, "Failed to record vendor assessment")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *VendorHandler) ListDueForAssessment(w http.ResponseWriter, r *http.Request) {
	horizon, limit, err := vendorHorizon(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid due-date query", err.Error())
		return
	}
	items, err := h.service.ListDueForAssessment(r.Context(), vendorOrgID(r), horizon, limit)
	if err != nil {
		writeVendorError(w, r, err, "Failed to list due vendor assessments")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (h *VendorHandler) ListDueContracts(w http.ResponseWriter, r *http.Request) {
	horizon, limit, err := vendorHorizon(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid due-date query", err.Error())
		return
	}
	items, err := h.service.ListDueContracts(r.Context(), vendorOrgID(r), horizon, limit)
	if err != nil {
		writeVendorError(w, r, err, "Failed to list due vendor contracts")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (h *VendorHandler) ListExpiringCertifications(w http.ResponseWriter, r *http.Request) {
	horizon, limit, err := vendorHorizon(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid due-date query", err.Error())
		return
	}
	items, err := h.service.ListExpiringCertifications(r.Context(), vendorOrgID(r), horizon, limit)
	if err != nil {
		writeVendorError(w, r, err, "Failed to list expiring vendor certifications")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (h *VendorHandler) ListEvents(w http.ResponseWriter, r *http.Request) {
	pagination := parsePagination(r)
	items, total, err := h.service.ListEvents(r.Context(), vendorOrgID(r), chi.URLParam(r, "id"), pagination)
	if err != nil {
		writeVendorError(w, r, err, "Failed to list vendor history")
		return
	}
	writePaginated(w, items, total, normalizedHandlerPagination(pagination))
}

func (h *VendorHandler) ListContacts(w http.ResponseWriter, r *http.Request) {
	h.writeRelated(w, r, "contacts")
}
func (h *VendorHandler) ListContracts(w http.ResponseWriter, r *http.Request) {
	h.writeRelated(w, r, "contracts")
}
func (h *VendorHandler) ListCertifications(w http.ResponseWriter, r *http.Request) {
	h.writeRelated(w, r, "certifications")
}
func (h *VendorHandler) ListSubprocessors(w http.ResponseWriter, r *http.Request) {
	h.writeRelated(w, r, "subprocessors")
}

func (h *VendorHandler) writeRelated(w http.ResponseWriter, r *http.Request, kind string) {
	item, err := h.service.GetByID(r.Context(), vendorOrgID(r), chi.URLParam(r, "id"))
	if err != nil {
		writeVendorError(w, r, err, "Failed to list vendor related records")
		return
	}
	var data any
	switch kind {
	case "contacts":
		data = item.Contacts
	case "contracts":
		data = item.Contracts
	case "certifications":
		data = item.CertificationDetails
	default:
		data = item.SubProcessors
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": data, "vendor_version": item.Version})
}

func (h *VendorHandler) SaveContact(w http.ResponseWriter, r *http.Request) {
	var input models.VendorContactInput
	if !decodeVendorInput(w, r, &input) {
		return
	}
	item, vendor, err := h.service.SaveContact(r.Context(), vendorOrgID(r), vendorUserID(r), chi.URLParam(r, "id"), chi.URLParam(r, "contactID"), input)
	writeVendorRelatedResult(w, r, item, vendor, err, "Failed to save vendor contact", chi.URLParam(r, "contactID") == "")
}
func (h *VendorHandler) DeleteContact(w http.ResponseWriter, r *http.Request) {
	h.deleteRelated(w, r, chi.URLParam(r, "contactID"), h.service.DeleteContact)
}
func (h *VendorHandler) SaveContract(w http.ResponseWriter, r *http.Request) {
	var input models.VendorContractInput
	if !decodeVendorInput(w, r, &input) {
		return
	}
	item, vendor, err := h.service.SaveContract(r.Context(), vendorOrgID(r), vendorUserID(r), chi.URLParam(r, "id"), chi.URLParam(r, "contractID"), input)
	writeVendorRelatedResult(w, r, item, vendor, err, "Failed to save vendor contract", chi.URLParam(r, "contractID") == "")
}
func (h *VendorHandler) DeleteContract(w http.ResponseWriter, r *http.Request) {
	h.deleteRelated(w, r, chi.URLParam(r, "contractID"), h.service.DeleteContract)
}
func (h *VendorHandler) SaveCertification(w http.ResponseWriter, r *http.Request) {
	var input models.VendorCertificationInput
	if !decodeVendorInput(w, r, &input) {
		return
	}
	item, vendor, err := h.service.SaveCertification(r.Context(), vendorOrgID(r), vendorUserID(r), chi.URLParam(r, "id"), chi.URLParam(r, "certificationID"), input)
	writeVendorRelatedResult(w, r, item, vendor, err, "Failed to save vendor certification", chi.URLParam(r, "certificationID") == "")
}
func (h *VendorHandler) DeleteCertification(w http.ResponseWriter, r *http.Request) {
	h.deleteRelated(w, r, chi.URLParam(r, "certificationID"), h.service.DeleteCertification)
}
func (h *VendorHandler) SaveSubprocessor(w http.ResponseWriter, r *http.Request) {
	var input models.VendorSubprocessorInput
	if !decodeVendorInput(w, r, &input) {
		return
	}
	item, vendor, err := h.service.SaveSubprocessor(r.Context(), vendorOrgID(r), vendorUserID(r), chi.URLParam(r, "id"), chi.URLParam(r, "subprocessorID"), input)
	writeVendorRelatedResult(w, r, item, vendor, err, "Failed to save vendor subprocessor", chi.URLParam(r, "subprocessorID") == "")
}
func (h *VendorHandler) DeleteSubprocessor(w http.ResponseWriter, r *http.Request) {
	h.deleteRelated(w, r, chi.URLParam(r, "subprocessorID"), h.service.DeleteSubprocessor)
}

type vendorRelatedDelete func(context.Context, string, string, string, string, int64) (*models.Vendor, error)

func (h *VendorHandler) deleteRelated(w http.ResponseWriter, r *http.Request, resourceID string, deleteFn vendorRelatedDelete) {
	version, err := requiredVendorVersion(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid version", err.Error())
		return
	}
	vendor, err := deleteFn(r.Context(), vendorOrgID(r), vendorUserID(r), chi.URLParam(r, "id"), resourceID, version)
	if err != nil {
		writeVendorError(w, r, err, "Failed to remove vendor related record")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"vendor": vendor})
}
func decodeVendorInput(w http.ResponseWriter, r *http.Request, target any) bool {
	if err := decodeAuditJSON(w, r, target); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return false
	}
	return true
}
func writeVendorRelatedResult(w http.ResponseWriter, r *http.Request, item, vendor any, err error, fallback string, created bool) {
	if err != nil {
		writeVendorError(w, r, err, fallback)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, map[string]any{"data": item, "vendor": vendor})
}

func vendorOrgID(r *http.Request) string  { return middleware.GetOrgIDFromContext(r.Context()) }
func vendorUserID(r *http.Request) string { return middleware.GetUserIDFromContext(r.Context()) }

func requiredVendorVersion(r *http.Request) (int64, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("version"))
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 1 {
		return 0, errors.New("version must be a positive integer")
	}
	return value, nil
}

func vendorHorizon(r *http.Request) (int, int, error) {
	parse := func(name string, fallback int) (int, error) {
		raw := strings.TrimSpace(r.URL.Query().Get(name))
		if raw == "" {
			return fallback, nil
		}
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 {
			return 0, errors.New(name + " must be a positive integer")
		}
		return value, nil
	}
	horizon, err := parse("horizon_days", 30)
	if err != nil {
		return 0, 0, err
	}
	limit, err := parse("limit", 100)
	return horizon, limit, err
}

func writeVendorError(w http.ResponseWriter, r *http.Request, err error, fallback string) {
	switch {
	case errors.Is(err, service.ErrSubscriptionLimitExceeded):
		writeError(w, http.StatusPaymentRequired, "Subscription vendor limit reached", "Upgrade the subscription or retire an existing vendor before creating another")
	case errors.Is(err, service.ErrVendorInvalid), errors.Is(err, service.ErrVendorInvalidID), errors.Is(err, service.ErrVendorInvalidTransition):
		writeError(w, http.StatusBadRequest, "Invalid vendor request", err.Error())
	case errors.Is(err, service.ErrVendorNotFound):
		writeError(w, http.StatusNotFound, "Vendor or related record not found", "")
	case errors.Is(err, service.ErrVendorUserNotFound):
		writeError(w, http.StatusUnprocessableEntity, "Vendor owner is not available", "")
	case errors.Is(err, service.ErrVendorConflict), errors.Is(err, service.ErrVendorVersionConflict):
		writeError(w, http.StatusConflict, "Vendor request conflicts with current state", err.Error())
	default:
		log.Error().Err(err).Str("request_id", middleware.GetRequestIDFromContext(r.Context())).Str("organization_id", vendorOrgID(r)).Msg("vendor request failed")
		writeError(w, http.StatusInternalServerError, fallback, "")
	}
}

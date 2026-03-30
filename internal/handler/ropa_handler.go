package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/complianceforge/platform/internal/middleware"
	"github.com/complianceforge/platform/internal/models"
)

// ---------- service interface ----------

// ROPAService defines the methods required by ROPAHandler for Records of Processing
// Activities and data privacy management.
type ROPAService interface {
	// Data classifications
	ListClassifications(ctx context.Context, orgID string, pagination models.PaginationRequest) ([]DataClassification, int, error)
	CreateClassification(ctx context.Context, orgID, userID string, classification *DataClassification) error

	// Data categories
	ListCategories(ctx context.Context, orgID string, pagination models.PaginationRequest) ([]DataCategory, int, error)
	CreateCategory(ctx context.Context, orgID, userID string, category *DataCategory) error

	// Processing activities
	ListProcessingActivities(ctx context.Context, orgID string, pagination models.PaginationRequest, filters ProcessingActivityFilters) ([]ProcessingActivity, int, error)
	CreateProcessingActivity(ctx context.Context, orgID, userID string, activity *ProcessingActivity) error
	GetProcessingActivity(ctx context.Context, orgID, activityID string) (*ProcessingActivityDetail, error)
	UpdateProcessingActivity(ctx context.Context, orgID string, activity *ProcessingActivity) error
	CreateDataFlows(ctx context.Context, orgID, activityID string, flows *DataFlowRequest) error
	GetFlowDiagram(ctx context.Context, orgID, activityID string) (*DataFlowDiagram, error)

	// ROPA export
	ExportROPA(ctx context.Context, orgID, userID string, req *ROPAExportRequest) (*ROPAExport, error)
	ListExports(ctx context.Context, orgID string, pagination models.PaginationRequest) ([]ROPAExport, int, error)
	DownloadExport(ctx context.Context, orgID, exportID string) (*ROPAExportFile, error)

	// Dashboard & analysis
	GetROPADashboard(ctx context.Context, orgID string) (*ROPADashboard, error)
	GetHighRiskActivities(ctx context.Context, orgID string) ([]ProcessingActivity, error)
	GetSubjectMap(ctx context.Context, orgID, category string) (*DataSubjectMap, error)
}

// ---------- request / response types ----------

// DataClassification represents a data classification level.
type DataClassification struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	Name           string `json:"name" validate:"required"`
	Description    string `json:"description"`
	Level          int    `json:"level"` // 1=public, 2=internal, 3=confidential, 4=restricted
	Color          string `json:"color,omitempty"`
	HandlingRules  string `json:"handling_rules,omitempty"`
	CreatedBy      string `json:"created_by"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

// DataCategory represents a personal data category (e.g., contact info, financial data).
type DataCategory struct {
	ID              string   `json:"id"`
	OrganizationID  string   `json:"organization_id"`
	Name            string   `json:"name" validate:"required"`
	Description     string   `json:"description"`
	IsSpecial       bool     `json:"is_special_category"` // GDPR Article 9
	ClassificationID string  `json:"classification_id,omitempty"`
	RetentionPeriod string   `json:"retention_period,omitempty"`
	LegalBases      []string `json:"legal_bases,omitempty"`
	CreatedBy       string   `json:"created_by"`
	CreatedAt       string   `json:"created_at"`
	UpdatedAt       string   `json:"updated_at"`
}

// ProcessingActivityFilters holds filter parameters for listing processing activities.
type ProcessingActivityFilters struct {
	LegalBasis   string `json:"legal_basis"`
	Department   string `json:"department"`
	RiskLevel    string `json:"risk_level"`
	Status       string `json:"status"`
	Search       string `json:"search"`
}

// ProcessingActivity represents a data processing activity for ROPA.
type ProcessingActivity struct {
	ID                  string   `json:"id"`
	OrganizationID      string   `json:"organization_id"`
	Name                string   `json:"name" validate:"required"`
	Description         string   `json:"description"`
	Purpose             string   `json:"purpose"`
	LegalBasis          string   `json:"legal_basis"` // consent, contract, legal_obligation, vital_interest, public_interest, legitimate_interest
	Department          string   `json:"department"`
	DataController      string   `json:"data_controller,omitempty"`
	DataProcessor       string   `json:"data_processor,omitempty"`
	DataCategoryIDs     []string `json:"data_category_ids,omitempty"`
	DataSubjectTypes    []string `json:"data_subject_types,omitempty"` // employees, customers, prospects, minors
	Recipients          []string `json:"recipients,omitempty"`
	ThirdCountryTransfers []string `json:"third_country_transfers,omitempty"`
	RetentionPeriod     string   `json:"retention_period,omitempty"`
	SecurityMeasures    []string `json:"security_measures,omitempty"`
	DPIARequired        bool     `json:"dpia_required"`
	DPIACompletedAt     string   `json:"dpia_completed_at,omitempty"`
	RiskLevel           string   `json:"risk_level,omitempty"` // high, medium, low
	Status              string   `json:"status"` // active, inactive, under_review
	CreatedBy           string   `json:"created_by"`
	CreatedAt           string   `json:"created_at"`
	UpdatedAt           string   `json:"updated_at"`
}

// ProcessingActivityDetail extends ProcessingActivity with data flows.
type ProcessingActivityDetail struct {
	ProcessingActivity
	DataFlows    []DataFlow     `json:"data_flows,omitempty"`
	DataCategories []DataCategory `json:"data_categories,omitempty"`
}

// DataFlow represents a data flow within a processing activity.
type DataFlow struct {
	ID          string `json:"id"`
	ActivityID  string `json:"activity_id"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
	DataType    string `json:"data_type"`
	Method      string `json:"method"`  // api, file_transfer, manual, automated
	Encrypted   bool   `json:"encrypted"`
	CrossBorder bool   `json:"cross_border"`
	Country     string `json:"country,omitempty"`
	Notes       string `json:"notes,omitempty"`
}

// DataFlowRequest is the payload for POST /data/processing-activities/{id}/flows.
type DataFlowRequest struct {
	Flows []DataFlow `json:"flows" validate:"required"`
}

// DataFlowDiagram represents a visual diagram of data flows for an activity.
type DataFlowDiagram struct {
	ActivityID string          `json:"activity_id"`
	Nodes      []FlowNode      `json:"nodes"`
	Edges      []FlowEdge      `json:"edges"`
}

// FlowNode represents a node in a data flow diagram.
type FlowNode struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Type  string `json:"type"` // source, processor, destination, storage
}

// FlowEdge represents an edge in a data flow diagram.
type FlowEdge struct {
	Source      string `json:"source"`
	Target      string `json:"target"`
	Label       string `json:"label,omitempty"`
	Encrypted   bool   `json:"encrypted"`
	CrossBorder bool   `json:"cross_border"`
}

// ROPAExportRequest is the payload for POST /data/ropa/export.
type ROPAExportRequest struct {
	Format   string   `json:"format" validate:"required"` // pdf, csv, excel, json
	Scope    string   `json:"scope,omitempty"`             // all, department, legal_basis
	FilterBy string   `json:"filter_by,omitempty"`
}

// ROPAExport represents an export of ROPA data.
type ROPAExport struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	Format         string `json:"format"`
	Status         string `json:"status"` // pending, generating, completed, error
	FileURL        string `json:"file_url,omitempty"`
	FileSize       int64  `json:"file_size,omitempty"`
	GeneratedBy    string `json:"generated_by"`
	GeneratedAt    string `json:"generated_at,omitempty"`
	CreatedAt      string `json:"created_at"`
}

// ROPAExportFile holds the downloadable file data for an export.
type ROPAExportFile struct {
	ExportID    string `json:"export_id"`
	FileName    string `json:"file_name"`
	ContentType string `json:"content_type"`
	FileData    []byte `json:"-"`
	FileURL     string `json:"file_url"`
}

// ROPADashboard provides ROPA metrics for an organization.
type ROPADashboard struct {
	TotalActivities       int            `json:"total_activities"`
	ActiveActivities      int            `json:"active_activities"`
	HighRiskActivities    int            `json:"high_risk_activities"`
	DPIARequired          int            `json:"dpia_required"`
	DPIACompleted         int            `json:"dpia_completed"`
	ByLegalBasis          map[string]int `json:"by_legal_basis"`
	ByDepartment          map[string]int `json:"by_department"`
	SpecialCategories     int            `json:"special_categories_processed"`
	CrossBorderTransfers  int            `json:"cross_border_transfers"`
	TotalDataCategories   int            `json:"total_data_categories"`
	TotalClassifications  int            `json:"total_classifications"`
}

// DataSubjectMap provides a map of data subjects and their data for a category.
type DataSubjectMap struct {
	Category       string              `json:"category"`
	SubjectTypes   []SubjectTypeData   `json:"subject_types"`
}

// SubjectTypeData represents data held about a specific subject type.
type SubjectTypeData struct {
	Type            string   `json:"type"`
	ActivityCount   int      `json:"activity_count"`
	DataCategories  []string `json:"data_categories"`
	LegalBases      []string `json:"legal_bases"`
	RetentionPeriods []string `json:"retention_periods"`
}

// ---------- handler ----------

// ROPAHandler handles ROPA and data privacy management endpoints.
type ROPAHandler struct {
	svc ROPAService
}

// NewROPAHandler creates a new ROPAHandler with the given service.
func NewROPAHandler(svc ROPAService) *ROPAHandler {
	return &ROPAHandler{svc: svc}
}

// ListClassifications handles GET /data/classifications.
func (h *ROPAHandler) ListClassifications(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	pagination := parsePagination(r)

	classifications, total, err := h.svc.ListClassifications(r.Context(), orgID, pagination)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list data classifications", err.Error())
		return
	}

	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": classifications,
		"pagination": models.PaginationResponse{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}

// CreateClassification handles POST /data/classifications.
func (h *ROPAHandler) CreateClassification(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	var classification DataClassification
	if err := json.NewDecoder(r.Body).Decode(&classification); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if classification.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required", "")
		return
	}

	if err := h.svc.CreateClassification(r.Context(), orgID, userID, &classification); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create data classification", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, classification)
}

// ListCategories handles GET /data/categories.
func (h *ROPAHandler) ListCategories(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	pagination := parsePagination(r)

	categories, total, err := h.svc.ListCategories(r.Context(), orgID, pagination)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list data categories", err.Error())
		return
	}

	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": categories,
		"pagination": models.PaginationResponse{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}

// CreateCategory handles POST /data/categories.
func (h *ROPAHandler) CreateCategory(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	var category DataCategory
	if err := json.NewDecoder(r.Body).Decode(&category); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if category.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required", "")
		return
	}

	if err := h.svc.CreateCategory(r.Context(), orgID, userID, &category); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create data category", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, category)
}

// ListProcessingActivities handles GET /data/processing-activities.
func (h *ROPAHandler) ListProcessingActivities(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	pagination := parsePagination(r)

	filters := ProcessingActivityFilters{
		LegalBasis: r.URL.Query().Get("legal_basis"),
		Department: r.URL.Query().Get("department"),
		RiskLevel:  r.URL.Query().Get("risk_level"),
		Status:     r.URL.Query().Get("status"),
		Search:     r.URL.Query().Get("search"),
	}

	activities, total, err := h.svc.ListProcessingActivities(r.Context(), orgID, pagination, filters)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list processing activities", err.Error())
		return
	}

	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": activities,
		"pagination": models.PaginationResponse{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}

// CreateProcessingActivity handles POST /data/processing-activities.
func (h *ROPAHandler) CreateProcessingActivity(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	var activity ProcessingActivity
	if err := json.NewDecoder(r.Body).Decode(&activity); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if activity.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required", "")
		return
	}

	if err := h.svc.CreateProcessingActivity(r.Context(), orgID, userID, &activity); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create processing activity", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, activity)
}

// GetProcessingActivity handles GET /data/processing-activities/{id}.
func (h *ROPAHandler) GetProcessingActivity(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	activityID := chi.URLParam(r, "id")
	if activityID == "" {
		writeError(w, http.StatusBadRequest, "Missing activity ID", "")
		return
	}

	detail, err := h.svc.GetProcessingActivity(r.Context(), orgID, activityID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Processing activity not found", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, detail)
}

// UpdateProcessingActivity handles PUT /data/processing-activities/{id}.
func (h *ROPAHandler) UpdateProcessingActivity(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	activityID := chi.URLParam(r, "id")
	if activityID == "" {
		writeError(w, http.StatusBadRequest, "Missing activity ID", "")
		return
	}

	var activity ProcessingActivity
	if err := json.NewDecoder(r.Body).Decode(&activity); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	activity.ID = activityID
	activity.OrganizationID = orgID

	if err := h.svc.UpdateProcessingActivity(r.Context(), orgID, &activity); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update processing activity", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, activity)
}

// CreateDataFlows handles POST /data/processing-activities/{id}/flows.
func (h *ROPAHandler) CreateDataFlows(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	activityID := chi.URLParam(r, "id")
	if activityID == "" {
		writeError(w, http.StatusBadRequest, "Missing activity ID", "")
		return
	}

	var req DataFlowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if len(req.Flows) == 0 {
		writeError(w, http.StatusBadRequest, "flows is required", "")
		return
	}

	if err := h.svc.CreateDataFlows(r.Context(), orgID, activityID, &req); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create data flows", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"message": "Data flows created"})
}

// GetFlowDiagram handles GET /data/processing-activities/{id}/flow-diagram.
func (h *ROPAHandler) GetFlowDiagram(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	activityID := chi.URLParam(r, "id")
	if activityID == "" {
		writeError(w, http.StatusBadRequest, "Missing activity ID", "")
		return
	}

	diagram, err := h.svc.GetFlowDiagram(r.Context(), orgID, activityID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get flow diagram", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, diagram)
}

// ExportROPA handles POST /data/ropa/export.
func (h *ROPAHandler) ExportROPA(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	var req ROPAExportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if req.Format == "" {
		writeError(w, http.StatusBadRequest, "format is required", "")
		return
	}

	export, err := h.svc.ExportROPA(r.Context(), orgID, userID, &req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to export ROPA", err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, export)
}

// ListExports handles GET /data/ropa/exports.
func (h *ROPAHandler) ListExports(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	pagination := parsePagination(r)

	exports, total, err := h.svc.ListExports(r.Context(), orgID, pagination)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list ROPA exports", err.Error())
		return
	}

	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": exports,
		"pagination": models.PaginationResponse{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}

// DownloadExport handles GET /data/ropa/exports/{id}/download.
func (h *ROPAHandler) DownloadExport(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	exportID := chi.URLParam(r, "id")
	if exportID == "" {
		writeError(w, http.StatusBadRequest, "Missing export ID", "")
		return
	}

	file, err := h.svc.DownloadExport(r.Context(), orgID, exportID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Export not found", err.Error())
		return
	}

	if len(file.FileData) > 0 {
		w.Header().Set("Content-Type", file.ContentType)
		w.Header().Set("Content-Disposition", "attachment; filename=\""+file.FileName+"\"")
		w.WriteHeader(http.StatusOK)
		w.Write(file.FileData)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"file_url":  file.FileURL,
		"file_name": file.FileName,
	})
}

// GetDashboard handles GET /data/ropa/dashboard.
func (h *ROPAHandler) GetDashboard(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())

	dashboard, err := h.svc.GetROPADashboard(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get ROPA dashboard", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, dashboard)
}

// GetHighRisk handles GET /data/high-risk.
func (h *ROPAHandler) GetHighRisk(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())

	activities, err := h.svc.GetHighRiskActivities(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get high-risk activities", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": activities})
}

// GetSubjectMap handles GET /data/subject-map/{category}.
func (h *ROPAHandler) GetSubjectMap(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	category := chi.URLParam(r, "category")
	if category == "" {
		writeError(w, http.StatusBadRequest, "Missing category", "")
		return
	}

	subjectMap, err := h.svc.GetSubjectMap(r.Context(), orgID, category)
	if err != nil {
		writeError(w, http.StatusNotFound, "Subject map not found", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, subjectMap)
}

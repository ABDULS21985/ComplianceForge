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

// BIAService defines the methods required by BIAHandler for business impact analysis
// and business continuity operations.
type BIAService interface {
	// BIA processes
	ListProcesses(ctx context.Context, orgID string, pagination models.PaginationRequest, filters BIAProcessFilters) ([]BIAProcess, int, error)
	CreateProcess(ctx context.Context, orgID, userID string, process *BIAProcess) error
	GetProcess(ctx context.Context, orgID, processID string) (*BIAProcessDetail, error)
	UpdateProcess(ctx context.Context, orgID string, process *BIAProcess) error
	MapDependencies(ctx context.Context, orgID, processID string, deps *DependencyMap) error
	GetDependencyGraph(ctx context.Context, orgID, processID string) (*DependencyGraph, error)
	GetSinglePointsOfFailure(ctx context.Context, orgID string) ([]SinglePointOfFailure, error)
	GetBIAReport(ctx context.Context, orgID string) (*BIAReport, error)

	// BC scenarios
	ListScenarios(ctx context.Context, orgID string, pagination models.PaginationRequest) ([]BCScenario, int, error)
	CreateScenario(ctx context.Context, orgID, userID string, scenario *BCScenario) error

	// BC plans
	ListBCPlans(ctx context.Context, orgID string, pagination models.PaginationRequest) ([]BCPlan, int, error)
	CreateBCPlan(ctx context.Context, orgID, userID string, plan *BCPlan) error
	ApproveBCPlan(ctx context.Context, orgID, userID, planID string, req *ApproveBCPlanRequest) error

	// BC exercises
	ListExercises(ctx context.Context, orgID string, pagination models.PaginationRequest) ([]BCExercise, int, error)
	CreateExercise(ctx context.Context, orgID, userID string, exercise *BCExercise) error
	CompleteExercise(ctx context.Context, orgID, userID, exerciseID string, results *ExerciseResults) error

	// Dashboard
	GetBCDashboard(ctx context.Context, orgID string) (*BCDashboard, error)
}

// ---------- request / response types ----------

// BIAProcessFilters holds filter parameters for listing BIA processes.
type BIAProcessFilters struct {
	Criticality string `json:"criticality"`
	Department  string `json:"department"`
	Search      string `json:"search"`
}

// BIAProcess represents a business process for impact analysis.
type BIAProcess struct {
	ID             string  `json:"id"`
	OrganizationID string  `json:"organization_id"`
	Name           string  `json:"name" validate:"required"`
	Description    string  `json:"description"`
	Department     string  `json:"department"`
	Owner          string  `json:"owner,omitempty"`
	Criticality    string  `json:"criticality"` // critical, high, medium, low
	RTO            int     `json:"rto"`          // recovery time objective in hours
	RPO            int     `json:"rpo"`          // recovery point objective in hours
	MTPD           int     `json:"mtpd"`         // max tolerable period of disruption in hours
	RevenueImpact  float64 `json:"revenue_impact,omitempty"`
	OperationalImpact string `json:"operational_impact,omitempty"`
	ReputationalImpact string `json:"reputational_impact,omitempty"`
	RegulatoryImpact  string `json:"regulatory_impact,omitempty"`
	CreatedBy      string  `json:"created_by"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
}

// BIAProcessDetail extends BIAProcess with dependencies.
type BIAProcessDetail struct {
	BIAProcess
	Dependencies []ProcessDependency `json:"dependencies"`
	DependedOnBy []ProcessDependency `json:"depended_on_by"`
}

// ProcessDependency represents a dependency between two business processes.
type ProcessDependency struct {
	ProcessID   string `json:"process_id"`
	ProcessName string `json:"process_name"`
	Type        string `json:"type"` // upstream, downstream, shared_resource
	Criticality string `json:"criticality"`
	Description string `json:"description,omitempty"`
}

// DependencyMap is the payload for POST /bia/processes/{id}/dependencies.
type DependencyMap struct {
	Dependencies []DependencyEntry `json:"dependencies" validate:"required"`
}

// DependencyEntry is a single dependency mapping.
type DependencyEntry struct {
	TargetProcessID string `json:"target_process_id" validate:"required"`
	Type            string `json:"type" validate:"required"`
	Criticality     string `json:"criticality,omitempty"`
	Description     string `json:"description,omitempty"`
}

// DependencyGraph is the graph data for visualizing process dependencies.
type DependencyGraph struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

// GraphNode represents a node in the dependency graph.
type GraphNode struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Criticality string `json:"criticality"`
	Department  string `json:"department"`
}

// GraphEdge represents an edge in the dependency graph.
type GraphEdge struct {
	Source      string `json:"source"`
	Target      string `json:"target"`
	Type        string `json:"type"`
	Criticality string `json:"criticality"`
}

// SinglePointOfFailure identifies a process that is a single point of failure.
type SinglePointOfFailure struct {
	ProcessID       string   `json:"process_id"`
	ProcessName     string   `json:"process_name"`
	Criticality     string   `json:"criticality"`
	DependentCount  int      `json:"dependent_count"`
	DependentNames  []string `json:"dependent_names"`
	RiskLevel       string   `json:"risk_level"`
	Recommendation  string   `json:"recommendation"`
}

// BIAReport is the full BIA report data.
type BIAReport struct {
	OrganizationID     string                `json:"organization_id"`
	TotalProcesses     int                   `json:"total_processes"`
	CriticalProcesses  int                   `json:"critical_processes"`
	AverageRTO         float64               `json:"average_rto_hours"`
	AverageRPO         float64               `json:"average_rpo_hours"`
	ByCriticality      map[string]int        `json:"by_criticality"`
	ByDepartment       map[string]int        `json:"by_department"`
	SinglePointsOfFailure []SinglePointOfFailure `json:"single_points_of_failure"`
	GeneratedAt        string                `json:"generated_at"`
}

// BCScenario represents a business continuity scenario.
type BCScenario struct {
	ID             string   `json:"id"`
	OrganizationID string   `json:"organization_id"`
	Name           string   `json:"name" validate:"required"`
	Description    string   `json:"description"`
	Type           string   `json:"type"` // natural_disaster, cyber_attack, pandemic, infrastructure_failure, supply_chain
	Likelihood     string   `json:"likelihood"` // very_high, high, medium, low, very_low
	Impact         string   `json:"impact"`     // catastrophic, major, moderate, minor, insignificant
	AffectedProcesses []string `json:"affected_processes,omitempty"`
	CreatedBy      string   `json:"created_by"`
	CreatedAt      string   `json:"created_at"`
	UpdatedAt      string   `json:"updated_at"`
}

// BCPlan represents a business continuity plan.
type BCPlan struct {
	ID             string       `json:"id"`
	OrganizationID string       `json:"organization_id"`
	Name           string       `json:"name" validate:"required"`
	Description    string       `json:"description"`
	ScenarioID     string       `json:"scenario_id,omitempty"`
	Status         string       `json:"status"` // draft, pending_approval, approved, active, archived
	Version        int          `json:"version"`
	Procedures     []BCProcedure `json:"procedures,omitempty"`
	ApprovedBy     string       `json:"approved_by,omitempty"`
	ApprovedAt     string       `json:"approved_at,omitempty"`
	LastTestedAt   string       `json:"last_tested_at,omitempty"`
	CreatedBy      string       `json:"created_by"`
	CreatedAt      string       `json:"created_at"`
	UpdatedAt      string       `json:"updated_at"`
}

// BCProcedure is a step in a BC plan.
type BCProcedure struct {
	Order       int    `json:"order"`
	Title       string `json:"title"`
	Description string `json:"description"`
	ResponsibleRole string `json:"responsible_role,omitempty"`
	TimeframeHours int    `json:"timeframe_hours,omitempty"`
}

// ApproveBCPlanRequest is the payload for POST /bc/plans/{id}/approve.
type ApproveBCPlanRequest struct {
	Comments string `json:"comments,omitempty"`
}

// BCExercise represents a business continuity exercise/drill.
type BCExercise struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	Name           string `json:"name" validate:"required"`
	Type           string `json:"type"` // tabletop, walkthrough, simulation, full_scale
	PlanID         string `json:"plan_id,omitempty"`
	ScenarioID     string `json:"scenario_id,omitempty"`
	Status         string `json:"status"` // scheduled, in_progress, completed, cancelled
	ScheduledDate  string `json:"scheduled_date"`
	CompletedDate  string `json:"completed_date,omitempty"`
	Participants   []string `json:"participants,omitempty"`
	CreatedBy      string `json:"created_by"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

// ExerciseResults is the payload for PUT /bc/exercises/{id}/complete.
type ExerciseResults struct {
	Outcome         string   `json:"outcome"` // pass, partial_pass, fail
	RTOAchieved     bool     `json:"rto_achieved"`
	RPOAchieved     bool     `json:"rpo_achieved"`
	LessonsLearned  string   `json:"lessons_learned,omitempty"`
	Issues          []string `json:"issues,omitempty"`
	Recommendations []string `json:"recommendations,omitempty"`
	Notes           string   `json:"notes,omitempty"`
}

// BCDashboard provides business continuity metrics.
type BCDashboard struct {
	TotalScenarios      int            `json:"total_scenarios"`
	TotalPlans          int            `json:"total_plans"`
	ApprovedPlans       int            `json:"approved_plans"`
	TotalExercises      int            `json:"total_exercises"`
	ExercisesThisYear   int            `json:"exercises_this_year"`
	LastExerciseDate    string         `json:"last_exercise_date,omitempty"`
	PlansByStatus       map[string]int `json:"plans_by_status"`
	CriticalProcesses   int            `json:"critical_processes"`
	SinglePointsOfFailure int          `json:"single_points_of_failure"`
	AverageRTO          float64        `json:"average_rto_hours"`
}

// ---------- handler ----------

// BIAHandler handles business impact analysis and business continuity endpoints.
type BIAHandler struct {
	svc BIAService
}

// NewBIAHandler creates a new BIAHandler with the given service.
func NewBIAHandler(svc BIAService) *BIAHandler {
	return &BIAHandler{svc: svc}
}

// ListProcesses handles GET /bia/processes.
func (h *BIAHandler) ListProcesses(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	pagination := parsePagination(r)

	filters := BIAProcessFilters{
		Criticality: r.URL.Query().Get("criticality"),
		Department:  r.URL.Query().Get("department"),
		Search:      r.URL.Query().Get("search"),
	}

	processes, total, err := h.svc.ListProcesses(r.Context(), orgID, pagination, filters)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list BIA processes", err.Error())
		return
	}

	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": processes,
		"pagination": models.PaginationResponse{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}

// CreateProcess handles POST /bia/processes.
func (h *BIAHandler) CreateProcess(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	var process BIAProcess
	if err := json.NewDecoder(r.Body).Decode(&process); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if process.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required", "")
		return
	}

	if err := h.svc.CreateProcess(r.Context(), orgID, userID, &process); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create BIA process", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, process)
}

// GetProcess handles GET /bia/processes/{id}.
func (h *BIAHandler) GetProcess(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	processID := chi.URLParam(r, "id")
	if processID == "" {
		writeError(w, http.StatusBadRequest, "Missing process ID", "")
		return
	}

	detail, err := h.svc.GetProcess(r.Context(), orgID, processID)
	if err != nil {
		writeError(w, http.StatusNotFound, "BIA process not found", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, detail)
}

// UpdateProcess handles PUT /bia/processes/{id}.
func (h *BIAHandler) UpdateProcess(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	processID := chi.URLParam(r, "id")
	if processID == "" {
		writeError(w, http.StatusBadRequest, "Missing process ID", "")
		return
	}

	var process BIAProcess
	if err := json.NewDecoder(r.Body).Decode(&process); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	process.ID = processID
	process.OrganizationID = orgID

	if err := h.svc.UpdateProcess(r.Context(), orgID, &process); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update BIA process", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, process)
}

// MapDependencies handles POST /bia/processes/{id}/dependencies.
func (h *BIAHandler) MapDependencies(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	processID := chi.URLParam(r, "id")
	if processID == "" {
		writeError(w, http.StatusBadRequest, "Missing process ID", "")
		return
	}

	var deps DependencyMap
	if err := json.NewDecoder(r.Body).Decode(&deps); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if len(deps.Dependencies) == 0 {
		writeError(w, http.StatusBadRequest, "dependencies is required", "")
		return
	}

	if err := h.svc.MapDependencies(r.Context(), orgID, processID, &deps); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to map dependencies", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Dependencies mapped"})
}

// GetDependencyGraph handles GET /bia/processes/{id}/dependency-graph.
func (h *BIAHandler) GetDependencyGraph(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	processID := chi.URLParam(r, "id")
	if processID == "" {
		writeError(w, http.StatusBadRequest, "Missing process ID", "")
		return
	}

	graph, err := h.svc.GetDependencyGraph(r.Context(), orgID, processID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get dependency graph", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, graph)
}

// GetSinglePointsOfFailure handles GET /bia/single-points-of-failure.
func (h *BIAHandler) GetSinglePointsOfFailure(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())

	spofs, err := h.svc.GetSinglePointsOfFailure(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get single points of failure", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": spofs})
}

// GetBIAReport handles GET /bia/report.
func (h *BIAHandler) GetBIAReport(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())

	report, err := h.svc.GetBIAReport(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get BIA report", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, report)
}

// ListScenarios handles GET /bc/scenarios.
func (h *BIAHandler) ListScenarios(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	pagination := parsePagination(r)

	scenarios, total, err := h.svc.ListScenarios(r.Context(), orgID, pagination)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list BC scenarios", err.Error())
		return
	}

	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": scenarios,
		"pagination": models.PaginationResponse{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}

// CreateScenario handles POST /bc/scenarios.
func (h *BIAHandler) CreateScenario(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	var scenario BCScenario
	if err := json.NewDecoder(r.Body).Decode(&scenario); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if scenario.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required", "")
		return
	}

	if err := h.svc.CreateScenario(r.Context(), orgID, userID, &scenario); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create BC scenario", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, scenario)
}

// ListBCPlans handles GET /bc/plans.
func (h *BIAHandler) ListBCPlans(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	pagination := parsePagination(r)

	plans, total, err := h.svc.ListBCPlans(r.Context(), orgID, pagination)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list BC plans", err.Error())
		return
	}

	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": plans,
		"pagination": models.PaginationResponse{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}

// CreateBCPlan handles POST /bc/plans.
func (h *BIAHandler) CreateBCPlan(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	var plan BCPlan
	if err := json.NewDecoder(r.Body).Decode(&plan); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if plan.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required", "")
		return
	}

	if err := h.svc.CreateBCPlan(r.Context(), orgID, userID, &plan); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create BC plan", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, plan)
}

// ApproveBCPlan handles POST /bc/plans/{id}/approve.
func (h *BIAHandler) ApproveBCPlan(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	planID := chi.URLParam(r, "id")
	if planID == "" {
		writeError(w, http.StatusBadRequest, "Missing plan ID", "")
		return
	}

	var req ApproveBCPlanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if err := h.svc.ApproveBCPlan(r.Context(), orgID, userID, planID, &req); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to approve BC plan", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Plan approved"})
}

// ListExercises handles GET /bc/exercises.
func (h *BIAHandler) ListExercises(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	pagination := parsePagination(r)

	exercises, total, err := h.svc.ListExercises(r.Context(), orgID, pagination)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list BC exercises", err.Error())
		return
	}

	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": exercises,
		"pagination": models.PaginationResponse{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}

// CreateExercise handles POST /bc/exercises.
func (h *BIAHandler) CreateExercise(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	var exercise BCExercise
	if err := json.NewDecoder(r.Body).Decode(&exercise); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if exercise.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required", "")
		return
	}

	if err := h.svc.CreateExercise(r.Context(), orgID, userID, &exercise); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create BC exercise", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, exercise)
}

// CompleteExercise handles PUT /bc/exercises/{id}/complete.
func (h *BIAHandler) CompleteExercise(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	exerciseID := chi.URLParam(r, "id")
	if exerciseID == "" {
		writeError(w, http.StatusBadRequest, "Missing exercise ID", "")
		return
	}

	var results ExerciseResults
	if err := json.NewDecoder(r.Body).Decode(&results); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if results.Outcome == "" {
		writeError(w, http.StatusBadRequest, "outcome is required", "")
		return
	}

	if err := h.svc.CompleteExercise(r.Context(), orgID, userID, exerciseID, &results); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to complete exercise", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Exercise completed"})
}

// GetBCDashboard handles GET /bc/dashboard.
func (h *BIAHandler) GetBCDashboard(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())

	dashboard, err := h.svc.GetBCDashboard(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get BC dashboard", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": dashboard})
}

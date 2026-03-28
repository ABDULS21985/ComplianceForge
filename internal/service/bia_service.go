package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// ---------------------------------------------------------------------------
// Domain types
// ---------------------------------------------------------------------------

// BusinessProcess represents a business process subject to impact analysis.
type BusinessProcess struct {
	ID                string  `json:"id"`
	OrgID             string  `json:"organization_id"`
	ProcessRef        string  `json:"process_ref"`
	Name              string  `json:"name"`
	Description       string  `json:"description"`
	Owner             *string `json:"owner"`
	Department        string  `json:"department"`
	CriticalityLevel  string  `json:"criticality_level"` // critical, high, medium, low
	RTO               int     `json:"rto_hours"`         // Recovery Time Objective in hours
	RPO               int     `json:"rpo_hours"`         // Recovery Point Objective in hours
	MTPD              int     `json:"mtpd_hours"`        // Maximum Tolerable Period of Disruption
	RevenueImpactHour float64 `json:"revenue_impact_per_hour"`
	PeakPeriods       string  `json:"peak_periods"`
	Status            string  `json:"status"`
	CreatedAt         string  `json:"created_at"`
}

// BusinessProcessDetail includes dependencies and other related data.
type BusinessProcessDetail struct {
	BusinessProcess
	Dependencies []Dependency          `json:"dependencies"`
	Scenarios    []BIAScenario         `json:"scenarios"`
	ImpactScores map[string]int        `json:"impact_scores"`
}

// CreateBusinessProcessRequest holds input for creating a business process.
type CreateBusinessProcessRequest struct {
	Name              string  `json:"name"`
	Description       string  `json:"description"`
	Owner             *string `json:"owner"`
	Department        string  `json:"department"`
	CriticalityLevel  string  `json:"criticality_level"`
	RTOHours          int     `json:"rto_hours"`
	RPOHours          int     `json:"rpo_hours"`
	MTPDHours         int     `json:"mtpd_hours"`
	RevenueImpactHour float64 `json:"revenue_impact_per_hour"`
	PeakPeriods       string  `json:"peak_periods"`
}

// UpdateBusinessProcessRequest holds input for updating a business process.
type UpdateBusinessProcessRequest struct {
	Name              *string  `json:"name"`
	Description       *string  `json:"description"`
	Owner             *string  `json:"owner"`
	Department        *string  `json:"department"`
	CriticalityLevel  *string  `json:"criticality_level"`
	RTOHours          *int     `json:"rto_hours"`
	RPOHours          *int     `json:"rpo_hours"`
	MTPDHours         *int     `json:"mtpd_hours"`
	RevenueImpactHour *float64 `json:"revenue_impact_per_hour"`
	PeakPeriods       *string  `json:"peak_periods"`
}

// Dependency maps a process to its dependencies (systems, people, vendors, etc.).
type Dependency struct {
	ID             string `json:"id"`
	ProcessID      string `json:"process_id"`
	DependencyType string `json:"dependency_type"` // system, application, vendor, personnel, facility, data
	DependencyName string `json:"dependency_name"`
	DependencyRef  string `json:"dependency_ref"`
	CriticalityLevel string `json:"criticality_level"`
	Description    string `json:"description"`
	RecoveryPriority int  `json:"recovery_priority"`
}

// DependencyGraph is the full graph of process-to-dependency relationships.
type DependencyGraph struct {
	Nodes []DependencyNode `json:"nodes"`
	Edges []DependencyEdge `json:"edges"`
}

// DependencyNode is a node in the dependency graph.
type DependencyNode struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	NodeType string `json:"type"` // process, system, vendor, personnel, facility, data
	Criticality string `json:"criticality"`
}

// DependencyEdge connects two nodes in the dependency graph.
type DependencyEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Label  string `json:"label"`
}

// SinglePointOfFailure identifies entities that multiple critical processes depend on.
type SinglePointOfFailure struct {
	DependencyName   string   `json:"dependency_name"`
	DependencyType   string   `json:"dependency_type"`
	DependentProcesses []string `json:"dependent_processes"`
	ProcessCount     int      `json:"process_count"`
	CriticalCount    int      `json:"critical_count"`
	RiskLevel        string   `json:"risk_level"`
}

// BIAScenario represents a disruption scenario for impact analysis.
type BIAScenario struct {
	ID              string                 `json:"id"`
	OrgID           string                 `json:"organization_id"`
	Name            string                 `json:"name"`
	ScenarioType    string                 `json:"scenario_type"` // cyber_attack, natural_disaster, pandemic, supply_chain, power_outage, data_breach
	Description     string                 `json:"description"`
	Likelihood      string                 `json:"likelihood"`    // very_high, high, medium, low, very_low
	AffectedProcessIDs []string            `json:"affected_process_ids"`
	EstimatedDowntimeHours int             `json:"estimated_downtime_hours"`
	FinancialImpactEUR float64             `json:"financial_impact_eur"`
	ReputationalImpact string              `json:"reputational_impact"` // severe, significant, moderate, minor, negligible
	Assumptions     map[string]interface{} `json:"assumptions"`
	CreatedAt       string                 `json:"created_at"`
}

// CreateBIAScenarioRequest holds input for creating a BIA scenario.
type CreateBIAScenarioRequest struct {
	Name                   string                 `json:"name"`
	ScenarioType           string                 `json:"scenario_type"`
	Description            string                 `json:"description"`
	Likelihood             string                 `json:"likelihood"`
	AffectedProcessIDs     []string               `json:"affected_process_ids"`
	EstimatedDowntimeHours int                    `json:"estimated_downtime_hours"`
	FinancialImpactEUR     float64                `json:"financial_impact_eur"`
	ReputationalImpact     string                 `json:"reputational_impact"`
	Assumptions            map[string]interface{} `json:"assumptions"`
}

// ContinuityPlan is a business continuity plan tied to processes and scenarios.
type ContinuityPlan struct {
	ID               string                   `json:"id"`
	OrgID            string                   `json:"organization_id"`
	PlanRef          string                   `json:"plan_ref"`
	Name             string                   `json:"name"`
	Description      string                   `json:"description"`
	Status           string                   `json:"status"` // draft, approved, active, under_review, retired
	Version          int                      `json:"version"`
	ProcessIDs       []string                 `json:"process_ids"`
	ScenarioIDs      []string                 `json:"scenario_ids"`
	RecoverySteps    []map[string]interface{} `json:"recovery_steps"`
	CommunicationPlan map[string]interface{}  `json:"communication_plan"`
	Owner            *string                  `json:"owner"`
	LastTestedAt     *string                  `json:"last_tested_at"`
	NextReviewDate   *string                  `json:"next_review_date"`
	CreatedAt        string                   `json:"created_at"`
}

// CreateContinuityPlanRequest holds input for creating a continuity plan.
type CreateContinuityPlanRequest struct {
	Name              string                   `json:"name"`
	Description       string                   `json:"description"`
	ProcessIDs        []string                 `json:"process_ids"`
	ScenarioIDs       []string                 `json:"scenario_ids"`
	RecoverySteps     []map[string]interface{} `json:"recovery_steps"`
	CommunicationPlan map[string]interface{}   `json:"communication_plan"`
	Owner             *string                  `json:"owner"`
	NextReviewDate    *string                  `json:"next_review_date"`
}

// BCExercise represents a business continuity test exercise.
type BCExercise struct {
	ID              string  `json:"id"`
	OrgID           string  `json:"organization_id"`
	PlanID          string  `json:"plan_id"`
	ExerciseType    string  `json:"exercise_type"` // tabletop, walkthrough, simulation, full_test
	Name            string  `json:"name"`
	Description     string  `json:"description"`
	ScheduledDate   string  `json:"scheduled_date"`
	Status          string  `json:"status"` // scheduled, in_progress, completed, cancelled
	Facilitator     *string `json:"facilitator"`
	Participants    int     `json:"participants"`
	CreatedAt       string  `json:"created_at"`
}

// ScheduleBCExerciseRequest holds input for scheduling an exercise.
type ScheduleBCExerciseRequest struct {
	PlanID        string  `json:"plan_id"`
	ExerciseType  string  `json:"exercise_type"`
	Name          string  `json:"name"`
	Description   string  `json:"description"`
	ScheduledDate string  `json:"scheduled_date"`
	Facilitator   *string `json:"facilitator"`
}

// ExerciseResults holds the outcome of a completed exercise.
type ExerciseResults struct {
	ActualRecoveryTimeHours int                      `json:"actual_recovery_time_hours"`
	ObjectivesMet           bool                     `json:"objectives_met"`
	Findings                []map[string]interface{} `json:"findings"`
	Improvements            []map[string]interface{} `json:"improvements"`
	ParticipantCount        int                      `json:"participant_count"`
	OverallRating           string                   `json:"overall_rating"` // excellent, good, adequate, poor
	Notes                   string                   `json:"notes"`
}

// BCDashboard provides an overview of business continuity posture.
type BCDashboard struct {
	TotalProcesses       int     `json:"total_processes"`
	CriticalProcesses    int     `json:"critical_processes"`
	HighProcesses        int     `json:"high_processes"`
	ProcessesCovered     int     `json:"processes_covered_by_plan"`
	TotalPlans           int     `json:"total_plans"`
	ActivePlans          int     `json:"active_plans"`
	PlansTestedLast12M   int     `json:"plans_tested_last_12_months"`
	AvgRTOHours          float64 `json:"avg_rto_hours"`
	AvgRPOHours          float64 `json:"avg_rpo_hours"`
	TotalScenarios       int     `json:"total_scenarios"`
	SPOFCount            int     `json:"single_points_of_failure"`
	UpcomingExercises    int     `json:"upcoming_exercises"`
	TotalFinancialExposure float64 `json:"total_financial_exposure_eur"`
}

// ---------------------------------------------------------------------------
// Service
// ---------------------------------------------------------------------------

// BIAService manages Business Impact Analysis and Business Continuity.
type BIAService struct {
	pool *pgxpool.Pool
	bus  *EventBus
}

// NewBIAService creates a new BIAService.
func NewBIAService(pool *pgxpool.Pool, bus *EventBus) *BIAService {
	return &BIAService{pool: pool, bus: bus}
}

// CreateProcess creates a new business process with an auto-generated reference.
func (bs *BIAService) CreateProcess(ctx context.Context, orgID string, req CreateBusinessProcessRequest) (*BusinessProcess, error) {
	// Auto-generate BP-NNN reference.
	var seqNum int
	err := bs.pool.QueryRow(ctx, `
		SELECT COALESCE(MAX(
			CAST(SUBSTRING(process_ref FROM 'BP-(\d+)') AS INTEGER)
		), 0) + 1
		FROM bia_business_processes
		WHERE organization_id = $1
	`, orgID).Scan(&seqNum)
	if err != nil {
		seqNum = 1
	}
	processRef := fmt.Sprintf("BP-%03d", seqNum)

	critLevel := req.CriticalityLevel
	if critLevel == "" {
		critLevel = "medium"
	}

	var proc BusinessProcess
	err = bs.pool.QueryRow(ctx, `
		INSERT INTO bia_business_processes (
			organization_id, process_ref, name, description, owner,
			department, criticality_level, rto_hours, rpo_hours, mtpd_hours,
			revenue_impact_per_hour, peak_periods, status, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, 'active', NOW())
		RETURNING id, organization_id, process_ref, name, description,
			owner, department, criticality_level, rto_hours, rpo_hours, mtpd_hours,
			revenue_impact_per_hour, peak_periods, status,
			TO_CHAR(created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
	`, orgID, processRef, req.Name, req.Description, req.Owner,
		req.Department, critLevel, req.RTOHours, req.RPOHours, req.MTPDHours,
		req.RevenueImpactHour, req.PeakPeriods,
	).Scan(
		&proc.ID, &proc.OrgID, &proc.ProcessRef, &proc.Name, &proc.Description,
		&proc.Owner, &proc.Department, &proc.CriticalityLevel,
		&proc.RTO, &proc.RPO, &proc.MTPD,
		&proc.RevenueImpactHour, &proc.PeakPeriods, &proc.Status, &proc.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("creating business process: %w", err)
	}

	if bs.bus != nil {
		bs.bus.Publish(Event{
			Type:       "bia.process_created",
			Severity:   "low",
			OrgID:      orgID,
			EntityType: "business_process",
			EntityID:   proc.ID,
			EntityRef:  processRef,
			Data:       map[string]interface{}{"criticality": critLevel},
			Timestamp:  time.Now().UTC(),
		})
	}

	log.Info().Str("process_id", proc.ID).Str("ref", processRef).Msg("bia: process created")
	return &proc, nil
}

// GetProcess returns the full detail of a business process including dependencies.
func (bs *BIAService) GetProcess(ctx context.Context, orgID, processID string) (*BusinessProcessDetail, error) {
	var proc BusinessProcessDetail
	err := bs.pool.QueryRow(ctx, `
		SELECT id, organization_id, process_ref, name, description,
			owner, department, criticality_level, rto_hours, rpo_hours, mtpd_hours,
			revenue_impact_per_hour, COALESCE(peak_periods, ''), status,
			TO_CHAR(created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM bia_business_processes
		WHERE id = $1 AND organization_id = $2
	`, processID, orgID).Scan(
		&proc.ID, &proc.OrgID, &proc.ProcessRef, &proc.Name, &proc.Description,
		&proc.Owner, &proc.Department, &proc.CriticalityLevel,
		&proc.RTO, &proc.RPO, &proc.MTPD,
		&proc.RevenueImpactHour, &proc.PeakPeriods, &proc.Status, &proc.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("business process not found")
		}
		return nil, fmt.Errorf("querying process: %w", err)
	}

	// Fetch dependencies.
	depRows, err := bs.pool.Query(ctx, `
		SELECT id, process_id, dependency_type, dependency_name,
			COALESCE(dependency_ref, ''), criticality_level,
			COALESCE(description, ''), recovery_priority
		FROM bia_dependencies
		WHERE process_id = $1
		ORDER BY recovery_priority
	`, processID)
	if err != nil {
		return nil, fmt.Errorf("querying dependencies: %w", err)
	}
	defer depRows.Close()

	for depRows.Next() {
		var d Dependency
		if err := depRows.Scan(&d.ID, &d.ProcessID, &d.DependencyType, &d.DependencyName,
			&d.DependencyRef, &d.CriticalityLevel, &d.Description, &d.RecoveryPriority); err != nil {
			return nil, fmt.Errorf("scanning dependency: %w", err)
		}
		proc.Dependencies = append(proc.Dependencies, d)
	}

	// Fetch scenarios that affect this process.
	scRows, err := bs.pool.Query(ctx, `
		SELECT s.id, s.organization_id, s.name, s.scenario_type,
			s.description, s.likelihood, s.affected_process_ids,
			s.estimated_downtime_hours, COALESCE(s.financial_impact_eur, 0),
			COALESCE(s.reputational_impact, 'moderate'),
			s.assumptions,
			TO_CHAR(s.created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM bia_scenarios s
		WHERE s.organization_id = $1 AND s.affected_process_ids @> ARRAY[$2]::uuid[]
		ORDER BY s.created_at DESC
	`, orgID, processID)
	if err == nil {
		defer scRows.Close()
		for scRows.Next() {
			var sc BIAScenario
			var processIDs []string
			var assumptionsJSON []byte
			if err := scRows.Scan(&sc.ID, &sc.OrgID, &sc.Name, &sc.ScenarioType,
				&sc.Description, &sc.Likelihood, &processIDs,
				&sc.EstimatedDowntimeHours, &sc.FinancialImpactEUR,
				&sc.ReputationalImpact, &assumptionsJSON, &sc.CreatedAt); err == nil {
				sc.AffectedProcessIDs = processIDs
				if assumptionsJSON != nil {
					_ = json.Unmarshal(assumptionsJSON, &sc.Assumptions)
				}
				proc.Scenarios = append(proc.Scenarios, sc)
			}
		}
	}

	// Build impact scores.
	proc.ImpactScores = map[string]int{
		"financial":    bs.calcFinancialScore(proc.RevenueImpactHour, proc.RTO),
		"operational":  bs.calcCriticalityScore(proc.CriticalityLevel),
		"reputational": bs.calcCriticalityScore(proc.CriticalityLevel),
	}

	return &proc, nil
}

// ListProcesses returns paginated business processes.
func (bs *BIAService) ListProcesses(ctx context.Context, orgID string, page, pageSize int) ([]BusinessProcess, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	var total int
	err := bs.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM bia_business_processes WHERE organization_id = $1
	`, orgID).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting processes: %w", err)
	}

	rows, err := bs.pool.Query(ctx, `
		SELECT id, organization_id, process_ref, name, description,
			owner, department, criticality_level, rto_hours, rpo_hours, mtpd_hours,
			revenue_impact_per_hour, COALESCE(peak_periods, ''), status,
			TO_CHAR(created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM bia_business_processes
		WHERE organization_id = $1
		ORDER BY CASE criticality_level WHEN 'critical' THEN 1 WHEN 'high' THEN 2 WHEN 'medium' THEN 3 ELSE 4 END, name
		LIMIT $2 OFFSET $3
	`, orgID, pageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("querying processes: %w", err)
	}
	defer rows.Close()

	var processes []BusinessProcess
	for rows.Next() {
		var p BusinessProcess
		if err := rows.Scan(
			&p.ID, &p.OrgID, &p.ProcessRef, &p.Name, &p.Description,
			&p.Owner, &p.Department, &p.CriticalityLevel,
			&p.RTO, &p.RPO, &p.MTPD,
			&p.RevenueImpactHour, &p.PeakPeriods, &p.Status, &p.CreatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scanning process: %w", err)
		}
		processes = append(processes, p)
	}

	return processes, total, nil
}

// UpdateProcess updates an existing business process.
func (bs *BIAService) UpdateProcess(ctx context.Context, orgID, processID string, req UpdateBusinessProcessRequest) error {
	tag, err := bs.pool.Exec(ctx, `
		UPDATE bia_business_processes SET
			name = COALESCE($3, name),
			description = COALESCE($4, description),
			owner = COALESCE($5, owner),
			department = COALESCE($6, department),
			criticality_level = COALESCE($7, criticality_level),
			rto_hours = COALESCE($8, rto_hours),
			rpo_hours = COALESCE($9, rpo_hours),
			mtpd_hours = COALESCE($10, mtpd_hours),
			revenue_impact_per_hour = COALESCE($11, revenue_impact_per_hour),
			peak_periods = COALESCE($12, peak_periods),
			updated_at = NOW()
		WHERE id = $1 AND organization_id = $2
	`, processID, orgID,
		req.Name, req.Description, req.Owner, req.Department,
		req.CriticalityLevel, req.RTOHours, req.RPOHours, req.MTPDHours,
		req.RevenueImpactHour, req.PeakPeriods,
	)
	if err != nil {
		return fmt.Errorf("updating process: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("business process not found")
	}

	log.Info().Str("process_id", processID).Msg("bia: process updated")
	return nil
}

// MapDependencies sets the dependencies for a business process.
func (bs *BIAService) MapDependencies(ctx context.Context, orgID, processID string, deps []Dependency) error {
	// Verify process ownership.
	var exists bool
	err := bs.pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM bia_business_processes WHERE id = $1 AND organization_id = $2)
	`, processID, orgID).Scan(&exists)
	if err != nil || !exists {
		return fmt.Errorf("business process not found")
	}

	tx, err := bs.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Remove existing dependencies.
	_, _ = tx.Exec(ctx, `DELETE FROM bia_dependencies WHERE process_id = $1`, processID)

	// Insert new dependencies.
	for i, dep := range deps {
		_, err := tx.Exec(ctx, `
			INSERT INTO bia_dependencies (
				process_id, dependency_type, dependency_name, dependency_ref,
				criticality_level, description, recovery_priority, created_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
		`, processID, dep.DependencyType, dep.DependencyName, dep.DependencyRef,
			dep.CriticalityLevel, dep.Description, i+1)
		if err != nil {
			return fmt.Errorf("inserting dependency %d: %w", i+1, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	log.Info().Str("process_id", processID).Int("deps", len(deps)).Msg("bia: dependencies mapped")
	return nil
}

// GetDependencyGraph builds the full dependency graph for an organisation.
func (bs *BIAService) GetDependencyGraph(ctx context.Context, orgID string) (*DependencyGraph, error) {
	graph := &DependencyGraph{}

	// Fetch all processes as nodes.
	pRows, err := bs.pool.Query(ctx, `
		SELECT id, name, criticality_level
		FROM bia_business_processes
		WHERE organization_id = $1 AND status = 'active'
	`, orgID)
	if err != nil {
		return nil, fmt.Errorf("querying processes: %w", err)
	}
	defer pRows.Close()

	processIDs := make(map[string]bool)
	for pRows.Next() {
		var id, name, crit string
		if err := pRows.Scan(&id, &name, &crit); err != nil {
			return nil, fmt.Errorf("scanning process node: %w", err)
		}
		graph.Nodes = append(graph.Nodes, DependencyNode{
			ID:          id,
			Label:       name,
			NodeType:    "process",
			Criticality: crit,
		})
		processIDs[id] = true
	}

	// Fetch all dependencies and create dependency nodes + edges.
	dRows, err := bs.pool.Query(ctx, `
		SELECT d.id, d.process_id, d.dependency_type, d.dependency_name,
			d.criticality_level
		FROM bia_dependencies d
		JOIN bia_business_processes bp ON bp.id = d.process_id
		WHERE bp.organization_id = $1 AND bp.status = 'active'
	`, orgID)
	if err != nil {
		return nil, fmt.Errorf("querying dependencies: %w", err)
	}
	defer dRows.Close()

	depNodeMap := make(map[string]bool) // track unique dependency nodes
	for dRows.Next() {
		var id, processID, depType, depName, crit string
		if err := dRows.Scan(&id, &processID, &depType, &depName, &crit); err != nil {
			return nil, fmt.Errorf("scanning dependency: %w", err)
		}

		// Create node for the dependency if not already present.
		nodeKey := depType + ":" + depName
		if !depNodeMap[nodeKey] {
			graph.Nodes = append(graph.Nodes, DependencyNode{
				ID:          nodeKey,
				Label:       depName,
				NodeType:    depType,
				Criticality: crit,
			})
			depNodeMap[nodeKey] = true
		}

		// Edge from process to dependency.
		graph.Edges = append(graph.Edges, DependencyEdge{
			Source: processID,
			Target: nodeKey,
			Label:  depType,
		})
	}

	return graph, nil
}

// IdentifySinglePointsOfFailure finds dependencies on which multiple critical
// processes depend.
func (bs *BIAService) IdentifySinglePointsOfFailure(ctx context.Context, orgID string) ([]SinglePointOfFailure, error) {
	rows, err := bs.pool.Query(ctx, `
		SELECT d.dependency_name, d.dependency_type,
			ARRAY_AGG(DISTINCT bp.name) AS process_names,
			COUNT(DISTINCT bp.id)::int AS process_count,
			COUNT(DISTINCT bp.id) FILTER (WHERE bp.criticality_level IN ('critical', 'high'))::int AS critical_count
		FROM bia_dependencies d
		JOIN bia_business_processes bp ON bp.id = d.process_id
		WHERE bp.organization_id = $1 AND bp.status = 'active'
		GROUP BY d.dependency_name, d.dependency_type
		HAVING COUNT(DISTINCT bp.id) >= 2
		ORDER BY critical_count DESC, process_count DESC
	`, orgID)
	if err != nil {
		return nil, fmt.Errorf("querying SPOFs: %w", err)
	}
	defer rows.Close()

	var spofs []SinglePointOfFailure
	for rows.Next() {
		var spof SinglePointOfFailure
		if err := rows.Scan(&spof.DependencyName, &spof.DependencyType,
			&spof.DependentProcesses, &spof.ProcessCount, &spof.CriticalCount); err != nil {
			return nil, fmt.Errorf("scanning SPOF: %w", err)
		}

		// Determine risk level.
		spof.RiskLevel = "low"
		if spof.CriticalCount >= 3 {
			spof.RiskLevel = "critical"
		} else if spof.CriticalCount >= 2 {
			spof.RiskLevel = "high"
		} else if spof.ProcessCount >= 3 {
			spof.RiskLevel = "medium"
		}

		spofs = append(spofs, spof)
	}

	return spofs, nil
}

// CreateScenario creates a disruption scenario for BIA.
func (bs *BIAService) CreateScenario(ctx context.Context, orgID string, req CreateBIAScenarioRequest) (*BIAScenario, error) {
	assumptionsJSON, _ := json.Marshal(req.Assumptions)

	var scenario BIAScenario
	err := bs.pool.QueryRow(ctx, `
		INSERT INTO bia_scenarios (
			organization_id, name, scenario_type, description,
			likelihood, affected_process_ids, estimated_downtime_hours,
			financial_impact_eur, reputational_impact, assumptions, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())
		RETURNING id, organization_id, name, scenario_type, description,
			likelihood, affected_process_ids, estimated_downtime_hours,
			financial_impact_eur, reputational_impact,
			TO_CHAR(created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
	`, orgID, req.Name, req.ScenarioType, req.Description,
		req.Likelihood, req.AffectedProcessIDs, req.EstimatedDowntimeHours,
		req.FinancialImpactEUR, req.ReputationalImpact, assumptionsJSON,
	).Scan(
		&scenario.ID, &scenario.OrgID, &scenario.Name, &scenario.ScenarioType,
		&scenario.Description, &scenario.Likelihood, &scenario.AffectedProcessIDs,
		&scenario.EstimatedDowntimeHours, &scenario.FinancialImpactEUR,
		&scenario.ReputationalImpact, &scenario.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("creating scenario: %w", err)
	}
	scenario.Assumptions = req.Assumptions

	log.Info().Str("scenario_id", scenario.ID).Str("type", scenario.ScenarioType).Msg("bia: scenario created")
	return &scenario, nil
}

// ListScenarios returns all scenarios for an organisation.
func (bs *BIAService) ListScenarios(ctx context.Context, orgID string) ([]BIAScenario, error) {
	rows, err := bs.pool.Query(ctx, `
		SELECT id, organization_id, name, scenario_type, description,
			likelihood, affected_process_ids, estimated_downtime_hours,
			COALESCE(financial_impact_eur, 0), COALESCE(reputational_impact, 'moderate'),
			assumptions,
			TO_CHAR(created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM bia_scenarios
		WHERE organization_id = $1
		ORDER BY created_at DESC
	`, orgID)
	if err != nil {
		return nil, fmt.Errorf("querying scenarios: %w", err)
	}
	defer rows.Close()

	var scenarios []BIAScenario
	for rows.Next() {
		var s BIAScenario
		var assumptionsJSON []byte
		if err := rows.Scan(&s.ID, &s.OrgID, &s.Name, &s.ScenarioType,
			&s.Description, &s.Likelihood, &s.AffectedProcessIDs,
			&s.EstimatedDowntimeHours, &s.FinancialImpactEUR,
			&s.ReputationalImpact, &assumptionsJSON, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning scenario: %w", err)
		}
		if assumptionsJSON != nil {
			_ = json.Unmarshal(assumptionsJSON, &s.Assumptions)
		}
		scenarios = append(scenarios, s)
	}

	return scenarios, nil
}

// CreateContinuityPlan creates a new business continuity plan.
func (bs *BIAService) CreateContinuityPlan(ctx context.Context, orgID string, req CreateContinuityPlanRequest) (*ContinuityPlan, error) {
	// Auto-generate plan reference.
	var seqNum int
	err := bs.pool.QueryRow(ctx, `
		SELECT COALESCE(MAX(
			CAST(SUBSTRING(plan_ref FROM 'BCP-(\d+)') AS INTEGER)
		), 0) + 1
		FROM bia_continuity_plans
		WHERE organization_id = $1
	`, orgID).Scan(&seqNum)
	if err != nil {
		seqNum = 1
	}
	planRef := fmt.Sprintf("BCP-%03d", seqNum)

	stepsJSON, _ := json.Marshal(req.RecoverySteps)
	commJSON, _ := json.Marshal(req.CommunicationPlan)

	var plan ContinuityPlan
	err = bs.pool.QueryRow(ctx, `
		INSERT INTO bia_continuity_plans (
			organization_id, plan_ref, name, description, status, version,
			process_ids, scenario_ids, recovery_steps, communication_plan,
			owner, next_review_date, created_at
		) VALUES ($1, $2, $3, $4, 'draft', 1, $5, $6, $7, $8, $9, $10::date, NOW())
		RETURNING id, organization_id, plan_ref, name, description, status, version,
			process_ids, scenario_ids, owner,
			CASE WHEN next_review_date IS NOT NULL THEN TO_CHAR(next_review_date, 'YYYY-MM-DD') END,
			TO_CHAR(created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
	`, orgID, planRef, req.Name, req.Description,
		req.ProcessIDs, req.ScenarioIDs, stepsJSON, commJSON,
		req.Owner, req.NextReviewDate,
	).Scan(
		&plan.ID, &plan.OrgID, &plan.PlanRef, &plan.Name, &plan.Description,
		&plan.Status, &plan.Version, &plan.ProcessIDs, &plan.ScenarioIDs,
		&plan.Owner, &plan.NextReviewDate, &plan.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("creating continuity plan: %w", err)
	}
	plan.RecoverySteps = req.RecoverySteps
	plan.CommunicationPlan = req.CommunicationPlan

	log.Info().Str("plan_id", plan.ID).Str("ref", planRef).Msg("bia: continuity plan created")
	return &plan, nil
}

// ScheduleExercise schedules a business continuity test exercise.
func (bs *BIAService) ScheduleExercise(ctx context.Context, orgID string, req ScheduleBCExerciseRequest) (*BCExercise, error) {
	var exercise BCExercise
	err := bs.pool.QueryRow(ctx, `
		INSERT INTO bia_exercises (
			organization_id, plan_id, exercise_type, name, description,
			scheduled_date, status, facilitator, created_at
		) VALUES ($1, $2, $3, $4, $5, $6::date, 'scheduled', $7, NOW())
		RETURNING id, organization_id, plan_id, exercise_type, name, description,
			TO_CHAR(scheduled_date, 'YYYY-MM-DD'), status, facilitator,
			TO_CHAR(created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
	`, orgID, req.PlanID, req.ExerciseType, req.Name, req.Description,
		req.ScheduledDate, req.Facilitator,
	).Scan(
		&exercise.ID, &exercise.OrgID, &exercise.PlanID, &exercise.ExerciseType,
		&exercise.Name, &exercise.Description, &exercise.ScheduledDate,
		&exercise.Status, &exercise.Facilitator, &exercise.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scheduling exercise: %w", err)
	}

	log.Info().Str("exercise_id", exercise.ID).Str("type", exercise.ExerciseType).Msg("bia: exercise scheduled")
	return &exercise, nil
}

// CompleteExercise records the results of a completed exercise.
func (bs *BIAService) CompleteExercise(ctx context.Context, orgID, exerciseID string, results ExerciseResults) error {
	findingsJSON, _ := json.Marshal(results.Findings)
	improvementsJSON, _ := json.Marshal(results.Improvements)

	tag, err := bs.pool.Exec(ctx, `
		UPDATE bia_exercises SET
			status = 'completed',
			completed_at = NOW(),
			actual_recovery_time_hours = $3,
			objectives_met = $4,
			findings = $5,
			improvements = $6,
			participant_count = $7,
			overall_rating = $8,
			notes = $9,
			updated_at = NOW()
		WHERE id = $1 AND organization_id = $2 AND status IN ('scheduled', 'in_progress')
	`, exerciseID, orgID,
		results.ActualRecoveryTimeHours, results.ObjectivesMet,
		findingsJSON, improvementsJSON,
		results.ParticipantCount, results.OverallRating, results.Notes,
	)
	if err != nil {
		return fmt.Errorf("completing exercise: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("exercise not found or already completed")
	}

	// Update the continuity plan's last_tested_at.
	_, _ = bs.pool.Exec(ctx, `
		UPDATE bia_continuity_plans SET last_tested_at = NOW()
		WHERE id = (SELECT plan_id FROM bia_exercises WHERE id = $1)
	`, exerciseID)

	if bs.bus != nil {
		bs.bus.Publish(Event{
			Type:       "bia.exercise_completed",
			Severity:   "low",
			OrgID:      orgID,
			EntityType: "bc_exercise",
			EntityID:   exerciseID,
			Data: map[string]interface{}{
				"objectives_met": results.ObjectivesMet,
				"rating":         results.OverallRating,
			},
			Timestamp: time.Now().UTC(),
		})
	}

	log.Info().Str("exercise_id", exerciseID).Str("rating", results.OverallRating).Msg("bia: exercise completed")
	return nil
}

// GetBCDashboard returns an overview of business continuity posture.
func (bs *BIAService) GetBCDashboard(ctx context.Context, orgID string) (*BCDashboard, error) {
	dash := &BCDashboard{}

	// Process counts.
	err := bs.pool.QueryRow(ctx, `
		SELECT
			COUNT(*)::int,
			COUNT(*) FILTER (WHERE criticality_level = 'critical')::int,
			COUNT(*) FILTER (WHERE criticality_level = 'high')::int,
			COALESCE(AVG(rto_hours), 0)::float8,
			COALESCE(AVG(rpo_hours), 0)::float8
		FROM bia_business_processes
		WHERE organization_id = $1 AND status = 'active'
	`, orgID).Scan(&dash.TotalProcesses, &dash.CriticalProcesses, &dash.HighProcesses,
		&dash.AvgRTOHours, &dash.AvgRPOHours)
	if err != nil {
		return nil, fmt.Errorf("querying process stats: %w", err)
	}

	// Plans.
	err = bs.pool.QueryRow(ctx, `
		SELECT
			COUNT(*)::int,
			COUNT(*) FILTER (WHERE status = 'active')::int,
			COUNT(*) FILTER (WHERE last_tested_at >= NOW() - INTERVAL '12 months')::int
		FROM bia_continuity_plans
		WHERE organization_id = $1
	`, orgID).Scan(&dash.TotalPlans, &dash.ActivePlans, &dash.PlansTestedLast12M)
	if err != nil {
		return nil, fmt.Errorf("querying plan stats: %w", err)
	}

	// Processes covered by at least one plan.
	_ = bs.pool.QueryRow(ctx, `
		SELECT COUNT(DISTINCT unnest)::int
		FROM bia_continuity_plans, unnest(process_ids)
		WHERE organization_id = $1 AND status IN ('active', 'approved')
	`, orgID).Scan(&dash.ProcessesCovered)

	// Scenarios.
	_ = bs.pool.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM bia_scenarios WHERE organization_id = $1
	`, orgID).Scan(&dash.TotalScenarios)

	// Single points of failure count.
	_ = bs.pool.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM (
			SELECT d.dependency_name
			FROM bia_dependencies d
			JOIN bia_business_processes bp ON bp.id = d.process_id
			WHERE bp.organization_id = $1 AND bp.status = 'active'
			GROUP BY d.dependency_name, d.dependency_type
			HAVING COUNT(DISTINCT bp.id) >= 2
		) spofs
	`, orgID).Scan(&dash.SPOFCount)

	// Upcoming exercises.
	_ = bs.pool.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM bia_exercises
		WHERE organization_id = $1 AND status = 'scheduled' AND scheduled_date >= CURRENT_DATE
	`, orgID).Scan(&dash.UpcomingExercises)

	// Total financial exposure (sum of revenue_impact * rto for critical/high).
	_ = bs.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(revenue_impact_per_hour * rto_hours), 0)::float8
		FROM bia_business_processes
		WHERE organization_id = $1 AND status = 'active'
		  AND criticality_level IN ('critical', 'high')
	`, orgID).Scan(&dash.TotalFinancialExposure)

	return dash, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (bs *BIAService) calcFinancialScore(revenuePerHour float64, rtoHours int) int {
	impact := revenuePerHour * float64(rtoHours)
	switch {
	case impact >= 100000:
		return 5
	case impact >= 50000:
		return 4
	case impact >= 10000:
		return 3
	case impact >= 1000:
		return 2
	default:
		return 1
	}
}

func (bs *BIAService) calcCriticalityScore(level string) int {
	switch level {
	case "critical":
		return 5
	case "high":
		return 4
	case "medium":
		return 3
	case "low":
		return 2
	default:
		return 1
	}
}

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
// Errors
// ---------------------------------------------------------------------------

var (
	ErrBoardMemberNotFound  = fmt.Errorf("board member not found")
	ErrBoardMeetingNotFound = fmt.Errorf("board meeting not found")
	ErrBoardDecisionNotFound = fmt.Errorf("board decision not found")
	ErrInvalidBoardToken    = fmt.Errorf("invalid or expired board portal token")
)

// ---------------------------------------------------------------------------
// Domain types
// ---------------------------------------------------------------------------

// BoardMember represents a governance board member.
type BoardMember struct {
	ID              string  `json:"id"`
	OrgID           string  `json:"organization_id"`
	Name            string  `json:"name"`
	Email           string  `json:"email"`
	Role            string  `json:"role"` // chairperson, member, secretary, observer
	Title           string  `json:"title"`
	Department      string  `json:"department"`
	Expertise       string  `json:"expertise"`
	IsExternal      bool    `json:"is_external"`
	Status          string  `json:"status"` // active, inactive, removed
	AppointedDate   string  `json:"appointed_date"`
	TermEndDate     *string `json:"term_end_date"`
	TrainingStatus  string  `json:"training_status"` // compliant, overdue, pending
	LastTrainingDate *string `json:"last_training_date"`
	PortalTokenHash *string `json:"-"`
	CreatedAt       string  `json:"created_at"`
}

// CreateBoardMemberRequest holds input for adding a board member.
type CreateBoardMemberRequest struct {
	Name          string  `json:"name"`
	Email         string  `json:"email"`
	Role          string  `json:"role"`
	Title         string  `json:"title"`
	Department    string  `json:"department"`
	Expertise     string  `json:"expertise"`
	IsExternal    bool    `json:"is_external"`
	AppointedDate string  `json:"appointed_date"`
	TermEndDate   *string `json:"term_end_date"`
}

// BoardMeeting represents a governance board meeting.
type BoardMeeting struct {
	ID             string                   `json:"id"`
	OrgID          string                   `json:"organization_id"`
	MeetingRef     string                   `json:"meeting_ref"`
	Title          string                   `json:"title"`
	MeetingType    string                   `json:"meeting_type"` // regular, special, emergency, annual
	ScheduledDate  string                   `json:"scheduled_date"`
	StartTime      *string                  `json:"start_time"`
	EndTime        *string                  `json:"end_time"`
	Location       string                   `json:"location"`
	Status         string                   `json:"status"` // scheduled, in_progress, completed, cancelled
	AgendaItems    []map[string]interface{} `json:"agenda_items"`
	Attendees      []string                 `json:"attendees"`
	QuorumRequired int                      `json:"quorum_required"`
	QuorumMet      bool                     `json:"quorum_met"`
	Minutes        *string                  `json:"minutes"`
	CreatedBy      string                   `json:"created_by"`
	CreatedAt      string                   `json:"created_at"`
}

// CreateBoardMeetingRequest holds input for scheduling a meeting.
type CreateBoardMeetingRequest struct {
	Title          string                   `json:"title"`
	MeetingType    string                   `json:"meeting_type"`
	ScheduledDate  string                   `json:"scheduled_date"`
	StartTime      *string                  `json:"start_time"`
	Location       string                   `json:"location"`
	AgendaItems    []map[string]interface{} `json:"agenda_items"`
	Attendees      []string                 `json:"attendees"`
	QuorumRequired int                      `json:"quorum_required"`
	CreatedBy      string                   `json:"created_by"`
}

// BoardDecision records a decision made during a board meeting.
type BoardDecision struct {
	ID              string                 `json:"id"`
	OrgID           string                 `json:"organization_id"`
	MeetingID       string                 `json:"meeting_id"`
	DecisionRef     string                 `json:"decision_ref"`
	Title           string                 `json:"title"`
	Description     string                 `json:"description"`
	DecisionType    string                 `json:"decision_type"` // approval, directive, resolution, action_item
	Status          string                 `json:"status"`        // pending, approved, rejected, deferred, implemented
	VotesFor        int                    `json:"votes_for"`
	VotesAgainst    int                    `json:"votes_against"`
	VotesAbstain    int                    `json:"votes_abstain"`
	EntityType      *string                `json:"entity_type"` // risk, policy, incident, exception, budget
	EntityID        *string                `json:"entity_id"`
	AssignedTo      *string                `json:"assigned_to"`
	DueDate         *string                `json:"due_date"`
	FollowUpActions []map[string]interface{} `json:"follow_up_actions"`
	RecordedBy      string                 `json:"recorded_by"`
	CreatedAt       string                 `json:"created_at"`
}

// RecordDecisionRequest holds input for recording a board decision.
type RecordDecisionRequest struct {
	Title           string                   `json:"title"`
	Description     string                   `json:"description"`
	DecisionType    string                   `json:"decision_type"`
	Status          string                   `json:"status"`
	VotesFor        int                      `json:"votes_for"`
	VotesAgainst    int                      `json:"votes_against"`
	VotesAbstain    int                      `json:"votes_abstain"`
	EntityType      *string                  `json:"entity_type"`
	EntityID        *string                  `json:"entity_id"`
	AssignedTo      *string                  `json:"assigned_to"`
	DueDate         *string                  `json:"due_date"`
	FollowUpActions []map[string]interface{} `json:"follow_up_actions"`
	RecordedBy      string                   `json:"recorded_by"`
}

// BoardPack is the compiled materials for a board meeting.
type BoardPack struct {
	MeetingID        string                 `json:"meeting_id"`
	MeetingRef       string                 `json:"meeting_ref"`
	Title            string                 `json:"title"`
	GeneratedAt      string                 `json:"generated_at"`
	ComplianceSummary map[string]interface{} `json:"compliance_summary"`
	RiskDashboard    map[string]interface{} `json:"risk_dashboard"`
	IncidentSummary  map[string]interface{} `json:"incident_summary"`
	RegulatoryUpdates []map[string]interface{} `json:"regulatory_updates"`
	DecisionsPending []BoardDecision        `json:"decisions_pending"`
	PreviousDecisions []BoardDecision       `json:"previous_decisions"`
	Agenda           []map[string]interface{} `json:"agenda"`
	Attendees        []BoardMember          `json:"attendees"`
}

// BoardDashboard provides governance-level KPIs.
type BoardDashboard struct {
	ComplianceGauge    float64            `json:"compliance_gauge"`      // 0-100 overall compliance score
	RiskAppetite       map[string]interface{} `json:"risk_appetite"`
	IncidentSummary    map[string]int     `json:"incident_summary"`
	DecisionsPending   int                `json:"decisions_pending"`
	DecisionsOverdue   int                `json:"decisions_overdue"`
	RegulatoryHorizon  []map[string]interface{} `json:"regulatory_horizon"`
	TotalMembers       int                `json:"total_members"`
	TrainingCompliant  int                `json:"training_compliant"`
	MeetingsThisQuarter int              `json:"meetings_this_quarter"`
}

// NIS2GovernanceReport holds NIS2-specific board governance evidence.
type NIS2GovernanceReport struct {
	GeneratedAt        string                   `json:"generated_at"`
	Organization       string                   `json:"organization"`
	TrainingStatus     []BoardMemberTraining    `json:"training_status"`
	RiskMeasures       []map[string]interface{} `json:"risk_measures_approved"`
	BoardOversight     []map[string]interface{} `json:"board_oversight_evidence"`
	ComplianceScore    float64                  `json:"compliance_score"`
	MeetingFrequency   int                      `json:"meetings_last_12_months"`
	DecisionsRecorded  int                      `json:"decisions_recorded"`
}

// BoardMemberTraining tracks training completion for a member.
type BoardMemberTraining struct {
	MemberID        string  `json:"member_id"`
	MemberName      string  `json:"member_name"`
	Role            string  `json:"role"`
	TrainingStatus  string  `json:"training_status"`
	LastTrainingDate *string `json:"last_training_date"`
	DaysSinceTraining int   `json:"days_since_training"`
	IsCompliant     bool    `json:"is_compliant"`
}

// BoardReport represents a generated board report.
type BoardReport struct {
	ID          string  `json:"id"`
	OrgID       string  `json:"organization_id"`
	MeetingID   *string `json:"meeting_id"`
	ReportType  string  `json:"report_type"`
	Title       string  `json:"title"`
	Format      string  `json:"format"`
	FilePath    *string `json:"file_path"`
	GeneratedBy string  `json:"generated_by"`
	CreatedAt   string  `json:"created_at"`
}

// ---------------------------------------------------------------------------
// Service
// ---------------------------------------------------------------------------

// BoardService manages governance board operations and reporting.
type BoardService struct {
	pool         *pgxpool.Pool
	reportEngine *ReportEngine
}

// NewBoardService creates a new BoardService.
func NewBoardService(pool *pgxpool.Pool, reportEngine *ReportEngine) *BoardService {
	return &BoardService{pool: pool, reportEngine: reportEngine}
}

// ---------------------------------------------------------------------------
// Board Member CRUD
// ---------------------------------------------------------------------------

// CreateMember adds a new board member.
func (s *BoardService) CreateMember(ctx context.Context, orgID string, req CreateBoardMemberRequest) (*BoardMember, error) {
	var m BoardMember
	err := s.pool.QueryRow(ctx, `
		INSERT INTO board_members (
			organization_id, name, email, role, title, department,
			expertise, is_external, status, appointed_date, term_end_date, training_status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'active', $9, $10, 'pending')
		RETURNING id, organization_id, name, email, role, title, department,
			expertise, is_external, status, appointed_date, term_end_date,
			training_status, last_training_date, created_at`,
		orgID, req.Name, req.Email, req.Role, req.Title, req.Department,
		req.Expertise, req.IsExternal, req.AppointedDate, req.TermEndDate,
	).Scan(
		&m.ID, &m.OrgID, &m.Name, &m.Email, &m.Role, &m.Title, &m.Department,
		&m.Expertise, &m.IsExternal, &m.Status, &m.AppointedDate, &m.TermEndDate,
		&m.TrainingStatus, &m.LastTrainingDate, &m.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create board member: %w", err)
	}

	log.Info().Str("member_id", m.ID).Str("name", m.Name).Msg("board member created")
	return &m, nil
}

// UpdateMember updates an existing board member.
func (s *BoardService) UpdateMember(ctx context.Context, orgID, memberID string, req CreateBoardMemberRequest) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE board_members
		SET name = $1, email = $2, role = $3, title = $4, department = $5,
			expertise = $6, is_external = $7, term_end_date = $8, updated_at = NOW()
		WHERE id = $9 AND organization_id = $10`,
		req.Name, req.Email, req.Role, req.Title, req.Department,
		req.Expertise, req.IsExternal, req.TermEndDate, memberID, orgID)
	if err != nil {
		return fmt.Errorf("update member: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrBoardMemberNotFound
	}
	return nil
}

// RemoveMember soft-deletes a board member.
func (s *BoardService) RemoveMember(ctx context.Context, orgID, memberID string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE board_members SET status = 'removed', updated_at = NOW()
		WHERE id = $1 AND organization_id = $2`, memberID, orgID)
	if err != nil {
		return fmt.Errorf("remove member: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrBoardMemberNotFound
	}
	return nil
}

// ListMembers returns all active board members for an organization.
func (s *BoardService) ListMembers(ctx context.Context, orgID string) ([]BoardMember, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, organization_id, name, email, role, title, department,
			expertise, is_external, status, appointed_date, term_end_date,
			training_status, last_training_date, created_at
		FROM board_members
		WHERE organization_id = $1 AND status != 'removed'
		ORDER BY role, name`, orgID)
	if err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	defer rows.Close()
	return s.scanMembers(rows)
}

// ---------------------------------------------------------------------------
// Board Meeting CRUD
// ---------------------------------------------------------------------------

// CreateMeeting schedules a new board meeting.
func (s *BoardService) CreateMeeting(ctx context.Context, orgID string, req CreateBoardMeetingRequest) (*BoardMeeting, error) {
	agendaJSON, _ := json.Marshal(req.AgendaItems)
	attendeesJSON, _ := json.Marshal(req.Attendees)

	var m BoardMeeting
	var agenda, attendees []byte
	err := s.pool.QueryRow(ctx, `
		INSERT INTO board_meetings (
			organization_id, title, meeting_type, scheduled_date, start_time,
			location, status, agenda_items, attendees, quorum_required, created_by
		) VALUES ($1, $2, $3, $4, $5, $6, 'scheduled', $7, $8, $9, $10)
		RETURNING id, organization_id, meeting_ref, title, meeting_type, scheduled_date,
			start_time, end_time, location, status, agenda_items, attendees,
			quorum_required, quorum_met, minutes, created_by, created_at`,
		orgID, req.Title, req.MeetingType, req.ScheduledDate, req.StartTime,
		req.Location, agendaJSON, attendeesJSON, req.QuorumRequired, req.CreatedBy,
	).Scan(
		&m.ID, &m.OrgID, &m.MeetingRef, &m.Title, &m.MeetingType, &m.ScheduledDate,
		&m.StartTime, &m.EndTime, &m.Location, &m.Status, &agenda, &attendees,
		&m.QuorumRequired, &m.QuorumMet, &m.Minutes, &m.CreatedBy, &m.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create meeting: %w", err)
	}
	_ = json.Unmarshal(agenda, &m.AgendaItems)
	_ = json.Unmarshal(attendees, &m.Attendees)

	log.Info().Str("meeting_id", m.ID).Str("ref", m.MeetingRef).Msg("board meeting scheduled")
	return &m, nil
}

// UpdateMeeting updates an existing meeting.
func (s *BoardService) UpdateMeeting(ctx context.Context, orgID, meetingID string, req CreateBoardMeetingRequest) error {
	agendaJSON, _ := json.Marshal(req.AgendaItems)
	attendeesJSON, _ := json.Marshal(req.Attendees)

	tag, err := s.pool.Exec(ctx, `
		UPDATE board_meetings
		SET title = $1, meeting_type = $2, scheduled_date = $3, start_time = $4,
			location = $5, agenda_items = $6, attendees = $7, quorum_required = $8, updated_at = NOW()
		WHERE id = $9 AND organization_id = $10`,
		req.Title, req.MeetingType, req.ScheduledDate, req.StartTime,
		req.Location, agendaJSON, attendeesJSON, req.QuorumRequired, meetingID, orgID)
	if err != nil {
		return fmt.Errorf("update meeting: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrBoardMeetingNotFound
	}
	return nil
}

// CompleteMeeting marks a meeting as completed and sets end time.
func (s *BoardService) CompleteMeeting(ctx context.Context, orgID, meetingID string, minutes string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE board_meetings
		SET status = 'completed', end_time = NOW(), minutes = $1, updated_at = NOW()
		WHERE id = $2 AND organization_id = $3`,
		minutes, meetingID, orgID)
	if err != nil {
		return fmt.Errorf("complete meeting: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrBoardMeetingNotFound
	}
	return nil
}

// ListMeetings returns paginated board meetings.
func (s *BoardService) ListMeetings(ctx context.Context, orgID string, page, pageSize int) ([]BoardMeeting, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	var total int
	_ = s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM board_meetings WHERE organization_id = $1`, orgID).Scan(&total)

	rows, err := s.pool.Query(ctx, `
		SELECT id, organization_id, meeting_ref, title, meeting_type, scheduled_date,
			start_time, end_time, location, status, agenda_items, attendees,
			quorum_required, quorum_met, minutes, created_by, created_at
		FROM board_meetings WHERE organization_id = $1
		ORDER BY scheduled_date DESC
		LIMIT $2 OFFSET $3`, orgID, pageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list meetings: %w", err)
	}
	defer rows.Close()

	var result []BoardMeeting
	for rows.Next() {
		var m BoardMeeting
		var agenda, attendees []byte
		if err := rows.Scan(&m.ID, &m.OrgID, &m.MeetingRef, &m.Title, &m.MeetingType,
			&m.ScheduledDate, &m.StartTime, &m.EndTime, &m.Location, &m.Status,
			&agenda, &attendees, &m.QuorumRequired, &m.QuorumMet, &m.Minutes,
			&m.CreatedBy, &m.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan meeting: %w", err)
		}
		_ = json.Unmarshal(agenda, &m.AgendaItems)
		_ = json.Unmarshal(attendees, &m.Attendees)
		result = append(result, m)
	}
	return result, total, nil
}

// ---------------------------------------------------------------------------
// Decisions
// ---------------------------------------------------------------------------

// RecordDecision records a decision from a board meeting, links to an entity,
// creates follow-up actions, and updates the entity status if applicable.
func (s *BoardService) RecordDecision(ctx context.Context, orgID, meetingID string, req RecordDecisionRequest) (*BoardDecision, error) {
	// Verify meeting exists.
	var meetingStatus string
	err := s.pool.QueryRow(ctx, `
		SELECT status FROM board_meetings WHERE id = $1 AND organization_id = $2`,
		meetingID, orgID).Scan(&meetingStatus)
	if err != nil {
		return nil, ErrBoardMeetingNotFound
	}

	followUpJSON, _ := json.Marshal(req.FollowUpActions)

	var d BoardDecision
	var fu []byte
	err = s.pool.QueryRow(ctx, `
		INSERT INTO board_decisions (
			organization_id, meeting_id, title, description, decision_type,
			status, votes_for, votes_against, votes_abstain,
			entity_type, entity_id, assigned_to, due_date,
			follow_up_actions, recorded_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		RETURNING id, organization_id, meeting_id, decision_ref, title, description,
			decision_type, status, votes_for, votes_against, votes_abstain,
			entity_type, entity_id, assigned_to, due_date,
			follow_up_actions, recorded_by, created_at`,
		orgID, meetingID, req.Title, req.Description, req.DecisionType,
		req.Status, req.VotesFor, req.VotesAgainst, req.VotesAbstain,
		req.EntityType, req.EntityID, req.AssignedTo, req.DueDate,
		followUpJSON, req.RecordedBy,
	).Scan(
		&d.ID, &d.OrgID, &d.MeetingID, &d.DecisionRef, &d.Title, &d.Description,
		&d.DecisionType, &d.Status, &d.VotesFor, &d.VotesAgainst, &d.VotesAbstain,
		&d.EntityType, &d.EntityID, &d.AssignedTo, &d.DueDate,
		&fu, &d.RecordedBy, &d.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("record decision: %w", err)
	}
	_ = json.Unmarshal(fu, &d.FollowUpActions)

	// Update linked entity status if decision is approved.
	if req.EntityType != nil && req.EntityID != nil && req.Status == "approved" {
		switch *req.EntityType {
		case "policy":
			_, _ = s.pool.Exec(ctx, `
				UPDATE policies SET status = 'approved', approved_by = 'board', approved_at = NOW()
				WHERE id = $1 AND organization_id = $2`, *req.EntityID, orgID)
		case "risk":
			_, _ = s.pool.Exec(ctx, `
				UPDATE risks SET mitigation_status = 'board_approved', updated_at = NOW()
				WHERE id = $1 AND organization_id = $2`, *req.EntityID, orgID)
		case "exception":
			_, _ = s.pool.Exec(ctx, `
				UPDATE compliance_exceptions SET status = 'approved', approved_by = 'board', approved_at = NOW()
				WHERE id = $1 AND organization_id = $2`, *req.EntityID, orgID)
		}
	}

	// Create follow-up action items.
	for _, action := range req.FollowUpActions {
		actionTitle, _ := action["title"].(string)
		assignee, _ := action["assigned_to"].(string)
		dueDate, _ := action["due_date"].(string)
		_, _ = s.pool.Exec(ctx, `
			INSERT INTO board_action_items (
				organization_id, decision_id, title, assigned_to, due_date, status
			) VALUES ($1, $2, $3, $4, $5, 'open')`,
			orgID, d.ID, actionTitle, assignee, dueDate)
	}

	log.Info().
		Str("decision_id", d.ID).
		Str("ref", d.DecisionRef).
		Str("type", d.DecisionType).
		Msg("board decision recorded")

	return &d, nil
}

// ListDecisions returns decisions for a meeting or all org decisions.
func (s *BoardService) ListDecisions(ctx context.Context, orgID string, meetingID *string) ([]BoardDecision, error) {
	var rows pgx.Rows
	var err error

	if meetingID != nil {
		rows, err = s.pool.Query(ctx, `
			SELECT id, organization_id, meeting_id, decision_ref, title, description,
				decision_type, status, votes_for, votes_against, votes_abstain,
				entity_type, entity_id, assigned_to, due_date,
				follow_up_actions, recorded_by, created_at
			FROM board_decisions
			WHERE organization_id = $1 AND meeting_id = $2
			ORDER BY created_at DESC`, orgID, *meetingID)
	} else {
		rows, err = s.pool.Query(ctx, `
			SELECT id, organization_id, meeting_id, decision_ref, title, description,
				decision_type, status, votes_for, votes_against, votes_abstain,
				entity_type, entity_id, assigned_to, due_date,
				follow_up_actions, recorded_by, created_at
			FROM board_decisions
			WHERE organization_id = $1
			ORDER BY created_at DESC LIMIT 100`, orgID)
	}
	if err != nil {
		return nil, fmt.Errorf("list decisions: %w", err)
	}
	defer rows.Close()
	return s.scanDecisions(rows)
}

// ---------------------------------------------------------------------------
// Board Pack & Dashboard
// ---------------------------------------------------------------------------

// GenerateBoardPack compiles a comprehensive board pack for a meeting.
func (s *BoardService) GenerateBoardPack(ctx context.Context, orgID, meetingID string) (*BoardPack, error) {
	pack := &BoardPack{
		MeetingID:   meetingID,
		GeneratedAt: time.Now().Format(time.RFC3339),
	}

	// Meeting details.
	var agenda, attendeeIDs []byte
	err := s.pool.QueryRow(ctx, `
		SELECT meeting_ref, title, agenda_items, attendees
		FROM board_meetings WHERE id = $1 AND organization_id = $2`,
		meetingID, orgID).Scan(&pack.MeetingRef, &pack.Title, &agenda, &attendeeIDs)
	if err != nil {
		return nil, ErrBoardMeetingNotFound
	}
	_ = json.Unmarshal(agenda, &pack.Agenda)

	// Compliance summary.
	var compScore float64
	var totalControls, implementedControls int
	_ = s.pool.QueryRow(ctx, `
		SELECT COUNT(*),
			COUNT(*) FILTER (WHERE implementation_status = 'implemented'),
			COALESCE(COUNT(*) FILTER (WHERE implementation_status = 'implemented') * 100.0 / NULLIF(COUNT(*), 0), 0)
		FROM control_implementations WHERE organization_id = $1`, orgID).Scan(&totalControls, &implementedControls, &compScore)
	pack.ComplianceSummary = map[string]interface{}{
		"overall_score":        compScore,
		"total_controls":       totalControls,
		"implemented_controls": implementedControls,
	}

	// Risk dashboard.
	riskByLevel := make(map[string]int)
	rRows, _ := s.pool.Query(ctx, `
		SELECT risk_level, COUNT(*) FROM risks WHERE organization_id = $1
		GROUP BY risk_level`, orgID)
	if rRows != nil {
		for rRows.Next() {
			var lvl string
			var cnt int
			rRows.Scan(&lvl, &cnt)
			riskByLevel[lvl] = cnt
		}
		rRows.Close()
	}
	var totalRisks int
	_ = s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM risks WHERE organization_id = $1`, orgID).Scan(&totalRisks)
	pack.RiskDashboard = map[string]interface{}{
		"total_risks":  totalRisks,
		"by_level":     riskByLevel,
	}

	// Incident summary.
	incidentSummary := make(map[string]interface{})
	var openIncidents, closedIncidents int
	_ = s.pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE status IN ('open', 'investigating', 'containing')),
			COUNT(*) FILTER (WHERE status = 'closed')
		FROM incidents WHERE organization_id = $1`, orgID).Scan(&openIncidents, &closedIncidents)
	incidentSummary["open"] = openIncidents
	incidentSummary["closed"] = closedIncidents
	pack.IncidentSummary = incidentSummary

	// Regulatory updates.
	regRows, _ := s.pool.Query(ctx, `
		SELECT title, description, effective_date, impact_level
		FROM regulatory_updates
		WHERE organization_id = $1 AND status = 'pending_review'
		ORDER BY effective_date ASC LIMIT 10`, orgID)
	if regRows != nil {
		for regRows.Next() {
			var title, desc, effectiveDate, impactLevel string
			regRows.Scan(&title, &desc, &effectiveDate, &impactLevel)
			pack.RegulatoryUpdates = append(pack.RegulatoryUpdates, map[string]interface{}{
				"title":          title,
				"description":    desc,
				"effective_date": effectiveDate,
				"impact_level":   impactLevel,
			})
		}
		regRows.Close()
	}

	// Pending decisions.
	pendingRows, err := s.pool.Query(ctx, `
		SELECT id, organization_id, meeting_id, decision_ref, title, description,
			decision_type, status, votes_for, votes_against, votes_abstain,
			entity_type, entity_id, assigned_to, due_date,
			follow_up_actions, recorded_by, created_at
		FROM board_decisions
		WHERE organization_id = $1 AND status = 'pending'
		ORDER BY created_at DESC`, orgID)
	if err == nil {
		defer pendingRows.Close()
		pack.DecisionsPending, _ = s.scanDecisions(pendingRows)
	}

	// Attendee details.
	var ids []string
	_ = json.Unmarshal(attendeeIDs, &ids)
	if len(ids) > 0 {
		memRows, _ := s.pool.Query(ctx, `
			SELECT id, organization_id, name, email, role, title, department,
				expertise, is_external, status, appointed_date, term_end_date,
				training_status, last_training_date, created_at
			FROM board_members WHERE organization_id = $1 AND id = ANY($2)`,
			orgID, ids)
		if memRows != nil {
			pack.Attendees, _ = s.scanMembers(memRows)
			memRows.Close()
		}
	}

	// Store report record.
	_, _ = s.pool.Exec(ctx, `
		INSERT INTO board_reports (organization_id, meeting_id, report_type, title, format, generated_by)
		VALUES ($1, $2, 'board_pack', $3, 'pdf', 'system')`,
		orgID, meetingID, fmt.Sprintf("Board Pack - %s", pack.Title))

	log.Info().Str("meeting_id", meetingID).Msg("board pack generated")
	return pack, nil
}

// GetBoardDashboard returns governance-level KPIs for the board.
func (s *BoardService) GetBoardDashboard(ctx context.Context, orgID string) (*BoardDashboard, error) {
	dash := &BoardDashboard{
		IncidentSummary: make(map[string]int),
		RiskAppetite:    make(map[string]interface{}),
	}

	// Compliance gauge.
	_ = s.pool.QueryRow(ctx, `
		SELECT COALESCE(
			COUNT(*) FILTER (WHERE implementation_status = 'implemented') * 100.0 / NULLIF(COUNT(*), 0), 0)
		FROM control_implementations WHERE organization_id = $1`, orgID).Scan(&dash.ComplianceGauge)

	// Risk appetite - risks above tolerance.
	var criticalRisks, highRisks, totalRisks int
	_ = s.pool.QueryRow(ctx, `
		SELECT COUNT(*), COUNT(*) FILTER (WHERE risk_level = 'critical'),
			COUNT(*) FILTER (WHERE risk_level = 'high')
		FROM risks WHERE organization_id = $1`, orgID).Scan(&totalRisks, &criticalRisks, &highRisks)
	dash.RiskAppetite = map[string]interface{}{
		"total": totalRisks, "critical": criticalRisks, "high": highRisks,
	}

	// Incidents by severity.
	iRows, _ := s.pool.Query(ctx, `
		SELECT severity, COUNT(*) FROM incidents
		WHERE organization_id = $1 AND status != 'closed'
		GROUP BY severity`, orgID)
	if iRows != nil {
		for iRows.Next() {
			var sev string
			var cnt int
			iRows.Scan(&sev, &cnt)
			dash.IncidentSummary[sev] = cnt
		}
		iRows.Close()
	}

	// Decisions.
	_ = s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM board_decisions
		WHERE organization_id = $1 AND status = 'pending'`, orgID).Scan(&dash.DecisionsPending)
	_ = s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM board_decisions
		WHERE organization_id = $1 AND status = 'pending'
			AND due_date IS NOT NULL AND due_date < CURRENT_DATE`, orgID).Scan(&dash.DecisionsOverdue)

	// Regulatory horizon.
	regRows, _ := s.pool.Query(ctx, `
		SELECT title, effective_date, impact_level
		FROM regulatory_updates
		WHERE organization_id = $1 AND effective_date > CURRENT_DATE
		ORDER BY effective_date ASC LIMIT 5`, orgID)
	if regRows != nil {
		for regRows.Next() {
			var title, effectiveDate, impactLevel string
			regRows.Scan(&title, &effectiveDate, &impactLevel)
			dash.RegulatoryHorizon = append(dash.RegulatoryHorizon, map[string]interface{}{
				"title": title, "effective_date": effectiveDate, "impact_level": impactLevel,
			})
		}
		regRows.Close()
	}

	// Members & training.
	_ = s.pool.QueryRow(ctx, `
		SELECT COUNT(*), COUNT(*) FILTER (WHERE training_status = 'compliant')
		FROM board_members WHERE organization_id = $1 AND status = 'active'`,
		orgID).Scan(&dash.TotalMembers, &dash.TrainingCompliant)

	// Meetings this quarter.
	_ = s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM board_meetings
		WHERE organization_id = $1 AND status = 'completed'
			AND scheduled_date >= DATE_TRUNC('quarter', CURRENT_DATE)`,
		orgID).Scan(&dash.MeetingsThisQuarter)

	return dash, nil
}

// GenerateNIS2GovernanceReport generates a NIS2 Directive governance compliance report.
func (s *BoardService) GenerateNIS2GovernanceReport(ctx context.Context, orgID string) (*NIS2GovernanceReport, error) {
	report := &NIS2GovernanceReport{
		GeneratedAt: time.Now().Format(time.RFC3339),
	}

	// Org name.
	_ = s.pool.QueryRow(ctx, `SELECT name FROM organizations WHERE id = $1`, orgID).Scan(&report.Organization)

	// Training status for all board members.
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, role, training_status, last_training_date,
			COALESCE(CURRENT_DATE - last_training_date::DATE, 999)
		FROM board_members
		WHERE organization_id = $1 AND status = 'active'
		ORDER BY name`, orgID)
	if err != nil {
		return nil, fmt.Errorf("query training: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var t BoardMemberTraining
		var daysSince int
		if err := rows.Scan(&t.MemberID, &t.MemberName, &t.Role,
			&t.TrainingStatus, &t.LastTrainingDate, &daysSince); err != nil {
			return nil, fmt.Errorf("scan training: %w", err)
		}
		t.DaysSinceTraining = daysSince
		t.IsCompliant = t.TrainingStatus == "compliant" && daysSince <= 365
		report.TrainingStatus = append(report.TrainingStatus, t)
	}

	// Risk measures approved by board.
	rmRows, _ := s.pool.Query(ctx, `
		SELECT bd.title, bd.decision_ref, bd.status, bd.created_at
		FROM board_decisions bd
		WHERE bd.organization_id = $1 AND bd.entity_type = 'risk'
			AND bd.decision_type = 'approval'
		ORDER BY bd.created_at DESC`, orgID)
	if rmRows != nil {
		for rmRows.Next() {
			var title, ref, status, createdAt string
			rmRows.Scan(&title, &ref, &status, &createdAt)
			report.RiskMeasures = append(report.RiskMeasures, map[string]interface{}{
				"title": title, "decision_ref": ref, "status": status, "date": createdAt,
			})
		}
		rmRows.Close()
	}

	// Board oversight evidence: meetings and decisions count.
	_ = s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM board_meetings
		WHERE organization_id = $1 AND status = 'completed'
			AND scheduled_date >= CURRENT_DATE - INTERVAL '12 months'`,
		orgID).Scan(&report.MeetingFrequency)
	_ = s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM board_decisions WHERE organization_id = $1`,
		orgID).Scan(&report.DecisionsRecorded)

	// Board oversight evidence entries.
	ovRows, _ := s.pool.Query(ctx, `
		SELECT bm.meeting_ref, bm.title, bm.scheduled_date, bm.status,
			(SELECT COUNT(*) FROM board_decisions bd WHERE bd.meeting_id = bm.id)
		FROM board_meetings bm
		WHERE bm.organization_id = $1 AND bm.status = 'completed'
		ORDER BY bm.scheduled_date DESC LIMIT 12`, orgID)
	if ovRows != nil {
		for ovRows.Next() {
			var ref, title, date, status string
			var decCount int
			ovRows.Scan(&ref, &title, &date, &status, &decCount)
			report.BoardOversight = append(report.BoardOversight, map[string]interface{}{
				"meeting_ref": ref, "title": title, "date": date,
				"status": status, "decisions_count": decCount,
			})
		}
		ovRows.Close()
	}

	// Compliance score: percentage of NIS2 controls implemented.
	_ = s.pool.QueryRow(ctx, `
		SELECT COALESCE(
			COUNT(*) FILTER (WHERE ci.implementation_status = 'implemented') * 100.0 / NULLIF(COUNT(*), 0), 0)
		FROM control_implementations ci
		JOIN framework_controls fc ON ci.control_id = fc.id
		JOIN frameworks f ON fc.framework_id = f.id
		WHERE ci.organization_id = $1 AND f.code = 'NIS2'`, orgID).Scan(&report.ComplianceScore)

	// Store report record.
	_, _ = s.pool.Exec(ctx, `
		INSERT INTO board_reports (organization_id, report_type, title, format, generated_by)
		VALUES ($1, 'nis2_governance', 'NIS2 Governance Report', 'pdf', 'system')`, orgID)

	log.Info().Str("org_id", orgID).Float64("score", report.ComplianceScore).Msg("NIS2 governance report generated")
	return report, nil
}

// ValidateBoardPortalToken validates a portal token for external board member access.
func (s *BoardService) ValidateBoardPortalToken(ctx context.Context, token string) (*BoardMember, error) {
	tokenHash := hashToken(token)

	var m BoardMember
	err := s.pool.QueryRow(ctx, `
		SELECT id, organization_id, name, email, role, title, department,
			expertise, is_external, status, appointed_date, term_end_date,
			training_status, last_training_date, created_at
		FROM board_members
		WHERE portal_token_hash = $1 AND status = 'active' AND is_external = true`,
		tokenHash,
	).Scan(
		&m.ID, &m.OrgID, &m.Name, &m.Email, &m.Role, &m.Title, &m.Department,
		&m.Expertise, &m.IsExternal, &m.Status, &m.AppointedDate, &m.TermEndDate,
		&m.TrainingStatus, &m.LastTrainingDate, &m.CreatedAt,
	)
	if err != nil {
		return nil, ErrInvalidBoardToken
	}
	return &m, nil
}

// ListReports returns generated board reports for an organization.
func (s *BoardService) ListReports(ctx context.Context, orgID string, page, pageSize int) ([]BoardReport, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	var total int
	_ = s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM board_reports WHERE organization_id = $1`, orgID).Scan(&total)

	rows, err := s.pool.Query(ctx, `
		SELECT id, organization_id, meeting_id, report_type, title, format, file_path, generated_by, created_at
		FROM board_reports WHERE organization_id = $1
		ORDER BY created_at DESC LIMIT $2 OFFSET $3`, orgID, pageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list reports: %w", err)
	}
	defer rows.Close()

	var result []BoardReport
	for rows.Next() {
		var r BoardReport
		if err := rows.Scan(&r.ID, &r.OrgID, &r.MeetingID, &r.ReportType, &r.Title,
			&r.Format, &r.FilePath, &r.GeneratedBy, &r.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan report: %w", err)
		}
		result = append(result, r)
	}
	return result, total, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (s *BoardService) scanMembers(rows pgx.Rows) ([]BoardMember, error) {
	var result []BoardMember
	for rows.Next() {
		var m BoardMember
		if err := rows.Scan(&m.ID, &m.OrgID, &m.Name, &m.Email, &m.Role, &m.Title, &m.Department,
			&m.Expertise, &m.IsExternal, &m.Status, &m.AppointedDate, &m.TermEndDate,
			&m.TrainingStatus, &m.LastTrainingDate, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan member: %w", err)
		}
		result = append(result, m)
	}
	return result, nil
}

func (s *BoardService) scanDecisions(rows pgx.Rows) ([]BoardDecision, error) {
	var result []BoardDecision
	for rows.Next() {
		var d BoardDecision
		var fu []byte
		if err := rows.Scan(&d.ID, &d.OrgID, &d.MeetingID, &d.DecisionRef, &d.Title, &d.Description,
			&d.DecisionType, &d.Status, &d.VotesFor, &d.VotesAgainst, &d.VotesAbstain,
			&d.EntityType, &d.EntityID, &d.AssignedTo, &d.DueDate,
			&fu, &d.RecordedBy, &d.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan decision: %w", err)
		}
		_ = json.Unmarshal(fu, &d.FollowUpActions)
		result = append(result, d)
	}
	return result, nil
}

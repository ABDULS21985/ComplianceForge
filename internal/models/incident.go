package models

import (
	"encoding/json"
	"time"
)

type BreachAssessmentStatus string

const (
	BreachAssessmentPending       BreachAssessmentStatus = "pending"
	BreachAssessmentNotNotifiable BreachAssessmentStatus = "not_notifiable"
	BreachAssessmentNotifiable    BreachAssessmentStatus = "notifiable"
)

type IncidentAssignmentRole string

const (
	IncidentAssignmentPrimary      IncidentAssignmentRole = "primary"
	IncidentAssignmentInvestigator IncidentAssignmentRole = "investigator"
	IncidentAssignmentObserver     IncidentAssignmentRole = "observer"
)

// Incident is the canonical API projection for the incident register. Tenant
// and actor identifiers are assigned from authenticated context, never request
// JSON. Version protects concurrent responder mutations from lost updates.
type Incident struct {
	TenantModel
	IncidentRef              string                 `json:"incident_ref"`
	Title                    string                 `json:"title"`
	Description              string                 `json:"description"`
	Category                 string                 `json:"category"`
	Severity                 IncidentSeverity       `json:"severity"`
	Status                   IncidentStatus         `json:"status"`
	ReporterID               string                 `json:"reporter_id"`
	AssigneeID               *string                `json:"assignee_id,omitempty"`
	DetectedAt               *time.Time             `json:"detected_at,omitempty"`
	ReportedAt               time.Time              `json:"reported_at"`
	OccurredAt               *time.Time             `json:"occurred_at,omitempty"`
	TriagedAt                *time.Time             `json:"triaged_at,omitempty"`
	InvestigationStartedAt   *time.Time             `json:"investigation_started_at,omitempty"`
	ContainedAt              *time.Time             `json:"contained_at,omitempty"`
	ResolvedAt               *time.Time             `json:"resolved_at,omitempty"`
	ClosedAt                 *time.Time             `json:"closed_at,omitempty"`
	CancelledAt              *time.Time             `json:"cancelled_at,omitempty"`
	CancellationReason       *string                `json:"cancellation_reason,omitempty"`
	ReopenedAt               *time.Time             `json:"reopened_at,omitempty"`
	RootCause                string                 `json:"root_cause"`
	Impact                   string                 `json:"impact"`
	LessonsLearned           string                 `json:"lessons_learned"`
	RelatedAssetID           *string                `json:"related_asset_id,omitempty"`
	FollowupDate             *time.Time             `json:"followup_date,omitempty"`
	IsDataBreach             bool                   `json:"is_data_breach"`
	BreachAssessmentStatus   BreachAssessmentStatus `json:"breach_assessment_status"`
	IsBreachNotifiable       bool                   `json:"is_breach_notifiable"`
	BreachAssessmentReason   *string                `json:"breach_assessment_reason,omitempty"`
	BreachAssessedAt         *time.Time             `json:"breach_assessed_at,omitempty"`
	BreachAssessedBy         *string                `json:"breach_assessed_by,omitempty"`
	BreachAwarenessAt        *time.Time             `json:"breach_awareness_at,omitempty"`
	NotificationDeadline     *time.Time             `json:"notification_deadline,omitempty"`
	DataSubjectsAffected     *int                   `json:"data_subjects_affected,omitempty"`
	RecordsAffected          *int64                 `json:"records_affected,omitempty"`
	DataCategories           []string               `json:"data_categories"`
	SpecialCategoryData      bool                   `json:"special_category_data"`
	CrossBorder              bool                   `json:"cross_border"`
	BreachNature             string                 `json:"breach_nature"`
	LikelyConsequences       string                 `json:"likely_consequences"`
	MitigationMeasures       string                 `json:"mitigation_measures"`
	DPANotifiedAt            *time.Time             `json:"dpa_notified_at,omitempty"`
	DPANotificationReference *string                `json:"dpa_notification_reference,omitempty"`
	DPANotificationReason    *string                `json:"dpa_notification_reason,omitempty"`
	Version                  int64                  `json:"version"`
	RetentionUntil           *time.Time             `json:"retention_until,omitempty"`
	LegalHold                bool                   `json:"legal_hold"`
	Metadata                 json.RawMessage        `json:"metadata"`
	DeadlineState            string                 `json:"deadline_state,omitempty"`
	HoursRemaining           *float64               `json:"hours_remaining,omitempty"`
}

type IncidentCreateInput struct {
	Title          string           `json:"title"`
	Description    string           `json:"description"`
	Category       string           `json:"category"`
	Severity       IncidentSeverity `json:"severity"`
	DetectedAt     *time.Time       `json:"detected_at,omitempty"`
	OccurredAt     *time.Time       `json:"occurred_at,omitempty"`
	RelatedAssetID *string          `json:"related_asset_id,omitempty"`
	FollowupDate   *time.Time       `json:"followup_date,omitempty"`
	RetentionUntil *time.Time       `json:"retention_until,omitempty"`
	Metadata       json.RawMessage  `json:"metadata,omitempty"`
}

type IncidentPatch struct {
	Version        int64             `json:"version"`
	Title          *string           `json:"title,omitempty"`
	Description    *string           `json:"description,omitempty"`
	Category       *string           `json:"category,omitempty"`
	Severity       *IncidentSeverity `json:"severity,omitempty"`
	DetectedAt     *time.Time        `json:"detected_at,omitempty"`
	OccurredAt     *time.Time        `json:"occurred_at,omitempty"`
	ClearOccurred  bool              `json:"clear_occurred_at,omitempty"`
	RootCause      *string           `json:"root_cause,omitempty"`
	Impact         *string           `json:"impact,omitempty"`
	LessonsLearned *string           `json:"lessons_learned,omitempty"`
	RelatedAssetID *string           `json:"related_asset_id,omitempty"`
	ClearAsset     bool              `json:"clear_related_asset_id,omitempty"`
	FollowupDate   *time.Time        `json:"followup_date,omitempty"`
	ClearFollowup  bool              `json:"clear_followup_date,omitempty"`
	RetentionUntil *time.Time        `json:"retention_until,omitempty"`
	LegalHold      *bool             `json:"legal_hold,omitempty"`
	Metadata       json.RawMessage   `json:"metadata,omitempty"`
}

type IncidentListFilter struct {
	PaginationRequest
	Status     string
	Severity   string
	Category   string
	AssigneeID string
	Search     string
	Breach     *bool
	Sort       string
	Direction  string
}

type IncidentTransitionInput struct {
	Status  IncidentStatus `json:"status"`
	Reason  string         `json:"reason,omitempty"`
	Version int64          `json:"version"`
}

type IncidentEscalationInput struct {
	Severity IncidentSeverity `json:"severity"`
	Reason   string           `json:"reason"`
	Version  int64            `json:"version"`
}

type IncidentBreachAssessmentInput struct {
	Version              int64                  `json:"version"`
	Status               BreachAssessmentStatus `json:"status"`
	Reason               string                 `json:"reason"`
	IsDataBreach         bool                   `json:"is_data_breach"`
	AwarenessAt          *time.Time             `json:"awareness_at,omitempty"`
	DataSubjectsAffected *int                   `json:"data_subjects_affected,omitempty"`
	RecordsAffected      *int64                 `json:"records_affected,omitempty"`
	DataCategories       []string               `json:"data_categories,omitempty"`
	SpecialCategoryData  bool                   `json:"special_category_data"`
	CrossBorder          bool                   `json:"cross_border"`
	BreachNature         string                 `json:"breach_nature,omitempty"`
	LikelyConsequences   string                 `json:"likely_consequences,omitempty"`
	MitigationMeasures   string                 `json:"mitigation_measures,omitempty"`
}

type IncidentDPANotificationInput struct {
	Version        int64     `json:"version"`
	IdempotencyKey string    `json:"idempotency_key"`
	NotifiedAt     time.Time `json:"notified_at"`
	Reference      string    `json:"reference"`
	Reason         string    `json:"reason"`
}

type IncidentAssignmentInput struct {
	Version    int64                  `json:"version"`
	AssigneeID string                 `json:"assignee_id"`
	Role       IncidentAssignmentRole `json:"role"`
	Reason     string                 `json:"reason"`
}

type IncidentUnassignmentInput struct {
	Version int64  `json:"version"`
	Reason  string `json:"reason"`
}

type IncidentEvent struct {
	ID             string          `json:"id"`
	OrganizationID string          `json:"organization_id"`
	IncidentID     string          `json:"incident_id"`
	EventType      string          `json:"event_type"`
	ActorUserID    string          `json:"actor_user_id"`
	FromStatus     *IncidentStatus `json:"from_status,omitempty"`
	ToStatus       *IncidentStatus `json:"to_status,omitempty"`
	Summary        string          `json:"summary"`
	Details        json.RawMessage `json:"details"`
	OccurredAt     time.Time       `json:"occurred_at"`
	CreatedAt      time.Time       `json:"created_at"`
}

type IncidentAssignment struct {
	ID             string                 `json:"id"`
	OrganizationID string                 `json:"organization_id"`
	IncidentID     string                 `json:"incident_id"`
	AssigneeUserID string                 `json:"assignee_user_id"`
	Role           IncidentAssignmentRole `json:"role"`
	AssignedBy     string                 `json:"assigned_by"`
	Reason         string                 `json:"reason"`
	AssignedAt     time.Time              `json:"assigned_at"`
	UnassignedAt   *time.Time             `json:"unassigned_at,omitempty"`
	UnassignedBy   *string                `json:"unassigned_by,omitempty"`
	UnassignReason *string                `json:"unassign_reason,omitempty"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
}

type IncidentStatistics struct {
	Total             int                      `json:"total"`
	Active            int                      `json:"active"`
	OverdueBreaches   int                      `json:"overdue_breaches"`
	UrgentBreaches    int                      `json:"urgent_breaches"`
	NotifiedBreaches  int                      `json:"notified_breaches"`
	AverageResolution float64                  `json:"average_resolution_hours"`
	ByStatus          map[IncidentStatus]int   `json:"by_status"`
	BySeverity        map[IncidentSeverity]int `json:"by_severity"`
}

package models

import (
	"encoding/json"
	"time"
)

type DataGovernancePolicy struct {
	OrganizationID               string          `json:"organization_id"`
	PrimaryRegion                string          `json:"primary_region"`
	AllowedRegions               []string        `json:"allowed_regions"`
	CrossBorderTransferMode      string          `json:"cross_border_transfer_mode"`
	DefaultRetentionDays         int             `json:"default_retention_days"`
	DefaultArchiveAfterDays      *int            `json:"default_archive_after_days,omitempty"`
	DeletionGraceDays            int             `json:"deletion_grace_days"`
	DispositionApprovalMode      string          `json:"disposition_approval_mode"`
	RequireProcessorConfirmation bool            `json:"require_processor_confirmation"`
	LegalHoldEnabled             bool            `json:"legal_hold_enabled"`
	PolicyStatement              string          `json:"policy_statement"`
	Version                      int64           `json:"version"`
	Metadata                     json.RawMessage `json:"metadata"`
	CreatedBy                    string          `json:"created_by"`
	UpdatedBy                    string          `json:"updated_by"`
	CreatedAt                    time.Time       `json:"created_at"`
	UpdatedAt                    time.Time       `json:"updated_at"`
}

type DataGovernancePolicyInput struct {
	PrimaryRegion                string          `json:"primary_region"`
	AllowedRegions               []string        `json:"allowed_regions"`
	CrossBorderTransferMode      string          `json:"cross_border_transfer_mode"`
	DefaultRetentionDays         int             `json:"default_retention_days"`
	DefaultArchiveAfterDays      *int            `json:"default_archive_after_days,omitempty"`
	DeletionGraceDays            int             `json:"deletion_grace_days"`
	DispositionApprovalMode      string          `json:"disposition_approval_mode"`
	RequireProcessorConfirmation bool            `json:"require_processor_confirmation"`
	LegalHoldEnabled             bool            `json:"legal_hold_enabled"`
	PolicyStatement              string          `json:"policy_statement,omitempty"`
	Metadata                     json.RawMessage `json:"metadata,omitempty"`
	ExpectedVersion              *int64          `json:"expected_version,omitempty"`
	Reason                       string          `json:"reason"`
}

type RetentionSchedule struct {
	TenantModel
	Name               string     `json:"name"`
	Description        string     `json:"description"`
	RecordType         string     `json:"record_type"`
	DataClassification string     `json:"data_classification,omitempty"`
	Jurisdiction       string     `json:"jurisdiction,omitempty"`
	TriggerEvent       string     `json:"trigger_event"`
	LegalBasis         string     `json:"legal_basis"`
	RetentionDays      int        `json:"retention_days"`
	ArchiveAfterDays   *int       `json:"archive_after_days,omitempty"`
	DispositionAction  string     `json:"disposition_action"`
	ReviewRequired     bool       `json:"review_required"`
	Priority           int        `json:"priority"`
	Status             string     `json:"status"`
	EffectiveFrom      time.Time  `json:"effective_from"`
	EffectiveUntil     *time.Time `json:"effective_until,omitempty"`
	Version            int64      `json:"version"`
	CreatedBy          string     `json:"created_by"`
	UpdatedBy          string     `json:"updated_by"`
}

type RetentionScheduleInput struct {
	Name               string     `json:"name"`
	Description        string     `json:"description,omitempty"`
	RecordType         string     `json:"record_type"`
	DataClassification string     `json:"data_classification,omitempty"`
	Jurisdiction       string     `json:"jurisdiction,omitempty"`
	TriggerEvent       string     `json:"trigger_event"`
	LegalBasis         string     `json:"legal_basis"`
	RetentionDays      int        `json:"retention_days"`
	ArchiveAfterDays   *int       `json:"archive_after_days,omitempty"`
	DispositionAction  string     `json:"disposition_action"`
	ReviewRequired     bool       `json:"review_required"`
	Priority           int        `json:"priority"`
	Status             string     `json:"status"`
	EffectiveFrom      time.Time  `json:"effective_from"`
	EffectiveUntil     *time.Time `json:"effective_until,omitempty"`
	Reason             string     `json:"reason"`
}

type RetentionSchedulePatch struct {
	ExpectedVersion     int64      `json:"expected_version"`
	Name                *string    `json:"name,omitempty"`
	Description         *string    `json:"description,omitempty"`
	DataClassification  *string    `json:"data_classification,omitempty"`
	ClearClassification bool       `json:"clear_data_classification,omitempty"`
	Jurisdiction        *string    `json:"jurisdiction,omitempty"`
	ClearJurisdiction   bool       `json:"clear_jurisdiction,omitempty"`
	TriggerEvent        *string    `json:"trigger_event,omitempty"`
	LegalBasis          *string    `json:"legal_basis,omitempty"`
	RetentionDays       *int       `json:"retention_days,omitempty"`
	ArchiveAfterDays    *int       `json:"archive_after_days,omitempty"`
	ClearArchiveAfter   bool       `json:"clear_archive_after_days,omitempty"`
	DispositionAction   *string    `json:"disposition_action,omitempty"`
	ReviewRequired      *bool      `json:"review_required,omitempty"`
	Priority            *int       `json:"priority,omitempty"`
	Status              *string    `json:"status,omitempty"`
	EffectiveFrom       *time.Time `json:"effective_from,omitempty"`
	EffectiveUntil      *time.Time `json:"effective_until,omitempty"`
	ClearEffectiveUntil bool       `json:"clear_effective_until,omitempty"`
	Reason              string     `json:"reason"`
}

type RetentionScheduleFilter struct {
	PaginationRequest
	RecordType         string
	Status             string
	DataClassification string
	Jurisdiction       string
	Search             string
	SortBy             string
	SortDirection      string
}

type RecordRetentionAssignment struct {
	TenantModel
	ScheduleID         string     `json:"schedule_id"`
	RecordType         string     `json:"record_type"`
	RecordID           string     `json:"record_id"`
	DataClassification string     `json:"data_classification,omitempty"`
	Jurisdiction       string     `json:"jurisdiction,omitempty"`
	TriggerEvent       string     `json:"trigger_event"`
	RetentionStartedAt time.Time  `json:"retention_started_at"`
	ArchiveEligibleAt  *time.Time `json:"archive_eligible_at,omitempty"`
	DispositionDueAt   time.Time  `json:"disposition_due_at"`
	DispositionAction  string     `json:"disposition_action"`
	ReviewRequired     bool       `json:"review_required"`
	ReviewStatus       string     `json:"review_status"`
	ReviewedBy         *string    `json:"reviewed_by,omitempty"`
	ReviewedAt         *time.Time `json:"reviewed_at,omitempty"`
	ReviewReason       string     `json:"review_reason,omitempty"`
	State              string     `json:"state"`
	Source             string     `json:"source"`
	Reason             string     `json:"reason"`
	Version            int64      `json:"version"`
	CreatedBy          string     `json:"created_by"`
	DisposedAt         *time.Time `json:"disposed_at,omitempty"`
}

type RetentionAssignmentInput struct {
	ScheduleID         string    `json:"schedule_id"`
	RecordType         string    `json:"record_type"`
	RecordID           string    `json:"record_id"`
	DataClassification string    `json:"data_classification,omitempty"`
	Jurisdiction       string    `json:"jurisdiction,omitempty"`
	RetentionStartedAt time.Time `json:"retention_started_at"`
	Source             string    `json:"source,omitempty"`
	Reason             string    `json:"reason"`
}

type RetentionReviewInput struct {
	ExpectedVersion int64  `json:"expected_version"`
	Decision        string `json:"decision"`
	Reason          string `json:"reason"`
}

type RetentionException struct {
	TenantModel
	AssignmentID   string     `json:"assignment_id"`
	RequestedUntil time.Time  `json:"requested_until"`
	Reason         string     `json:"reason"`
	Status         string     `json:"status"`
	RequestedBy    string     `json:"requested_by"`
	DecidedBy      *string    `json:"decided_by,omitempty"`
	DecidedAt      *time.Time `json:"decided_at,omitempty"`
	DecisionReason string     `json:"decision_reason,omitempty"`
	Version        int64      `json:"version"`
}

type RetentionExceptionInput struct {
	RequestedUntil time.Time `json:"requested_until"`
	Reason         string    `json:"reason"`
}

type RetentionExceptionDecisionInput struct {
	ExpectedVersion int64  `json:"expected_version"`
	Decision        string `json:"decision"`
	Reason          string `json:"reason"`
}

type LegalHold struct {
	TenantModel
	HoldRef         string          `json:"hold_ref"`
	Name            string          `json:"name"`
	MatterReference string          `json:"matter_reference,omitempty"`
	Description     string          `json:"description"`
	LegalAuthority  string          `json:"legal_authority"`
	Scope           json.RawMessage `json:"scope"`
	Status          string          `json:"status"`
	OwnerUserID     string          `json:"owner_user_id"`
	PlacedBy        string          `json:"placed_by"`
	PlacedAt        time.Time       `json:"placed_at"`
	ReviewDueAt     *time.Time      `json:"review_due_at,omitempty"`
	ReleasedBy      *string         `json:"released_by,omitempty"`
	ReleasedAt      *time.Time      `json:"released_at,omitempty"`
	ReleaseReason   string          `json:"release_reason,omitempty"`
	Version         int64           `json:"version"`
	CustodianIDs    []string        `json:"custodian_ids"`
	RecordCount     int             `json:"record_count"`
}

type LegalHoldInput struct {
	Name            string          `json:"name"`
	MatterReference string          `json:"matter_reference,omitempty"`
	Description     string          `json:"description"`
	LegalAuthority  string          `json:"legal_authority"`
	Scope           json.RawMessage `json:"scope,omitempty"`
	OwnerUserID     string          `json:"owner_user_id"`
	ReviewDueAt     *time.Time      `json:"review_due_at,omitempty"`
	CustodianIDs    []string        `json:"custodian_ids,omitempty"`
	Reason          string          `json:"reason"`
}

type LegalHoldPatch struct {
	ExpectedVersion      int64            `json:"expected_version"`
	Name                 *string          `json:"name,omitempty"`
	MatterReference      *string          `json:"matter_reference,omitempty"`
	ClearMatterReference bool             `json:"clear_matter_reference,omitempty"`
	Description          *string          `json:"description,omitempty"`
	LegalAuthority       *string          `json:"legal_authority,omitempty"`
	Scope                *json.RawMessage `json:"scope,omitempty"`
	OwnerUserID          *string          `json:"owner_user_id,omitempty"`
	ReviewDueAt          *time.Time       `json:"review_due_at,omitempty"`
	ClearReviewDue       bool             `json:"clear_review_due_at,omitempty"`
	Reason               string           `json:"reason"`
}

type LegalHoldReleaseInput struct {
	ExpectedVersion int64  `json:"expected_version"`
	Outcome         string `json:"outcome"`
	Reason          string `json:"reason"`
}

type LegalHoldRecord struct {
	ID             string     `json:"id"`
	OrganizationID string     `json:"organization_id"`
	HoldID         string     `json:"hold_id"`
	RecordType     string     `json:"record_type"`
	RecordID       string     `json:"record_id"`
	Reason         string     `json:"reason"`
	PlacedBy       string     `json:"placed_by"`
	PlacedAt       time.Time  `json:"placed_at"`
	ReleasedAt     *time.Time `json:"released_at,omitempty"`
	ReleasedBy     *string    `json:"released_by,omitempty"`
	ReleaseReason  string     `json:"release_reason,omitempty"`
}

type LegalHoldRecordInput struct {
	RecordType string `json:"record_type"`
	RecordID   string `json:"record_id"`
	Reason     string `json:"reason"`
}

type LegalHoldRecordReleaseInput struct {
	Reason string `json:"reason"`
}

type DataGovernanceEvent struct {
	ID            string          `json:"id"`
	ChainSequence int64           `json:"chain_sequence"`
	PreviousHash  string          `json:"previous_hash"`
	EventHash     string          `json:"event_hash"`
	EntityType    string          `json:"entity_type"`
	EntityID      string          `json:"entity_id"`
	EventType     string          `json:"event_type"`
	ActorUserID   string          `json:"actor_user_id"`
	Reason        string          `json:"reason"`
	BeforeState   json.RawMessage `json:"before_state,omitempty"`
	AfterState    json.RawMessage `json:"after_state,omitempty"`
	Source        string          `json:"source"`
	RequestID     string          `json:"request_id,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
}

type DataGovernanceEventFilter struct {
	PaginationRequest
	EntityType string
	EntityID   string
	EventType  string
}

type GovernanceChainVerification struct {
	Valid         bool      `json:"valid"`
	EventCount    int64     `json:"event_count"`
	LastSequence  int64     `json:"last_sequence"`
	LastHash      string    `json:"last_hash"`
	VerifiedAt    time.Time `json:"verified_at"`
	FailureReason string    `json:"failure_reason,omitempty"`
}

type RecordDispositionDecision struct {
	RecordType       string                     `json:"record_type"`
	RecordID         string                     `json:"record_id"`
	Allowed          bool                       `json:"allowed"`
	Reason           string                     `json:"reason"`
	Assignment       *RecordRetentionAssignment `json:"assignment,omitempty"`
	ActiveLegalHolds []LegalHoldRecord          `json:"active_legal_holds"`
	EvaluatedAt      time.Time                  `json:"evaluated_at"`
}

package models

import (
	"encoding/json"
	"time"
)

type ControlPriority string

const (
	ControlPriorityCritical ControlPriority = "critical"
	ControlPriorityHigh     ControlPriority = "high"
	ControlPriorityMedium   ControlPriority = "medium"
	ControlPriorityLow      ControlPriority = "low"
)

type ImplementationStatus string

const (
	ImplementationStatusNotStarted ImplementationStatus = "not_started"
	ImplementationStatusInProgress ImplementationStatus = "in_progress"
	ImplementationStatusCompleted  ImplementationStatus = "completed"
	ImplementationStatusFailed     ImplementationStatus = "failed"
)

type ControlImplementationState string

const (
	ControlStateNotApplicable  ControlImplementationState = "not_applicable"
	ControlStateNotImplemented ControlImplementationState = "not_implemented"
	ControlStatePlanned        ControlImplementationState = "planned"
	ControlStatePartial        ControlImplementationState = "partial"
	ControlStateImplemented    ControlImplementationState = "implemented"
	ControlStateEffective      ControlImplementationState = "effective"
)

// Control is the immutable catalog definition plus the requesting tenant's
// implementation, when that framework has been adopted.
type Control struct {
	BaseModel
	FrameworkID          string                 `json:"framework_id"`
	DomainID             *string                `json:"domain_id,omitempty"`
	Code                 string                 `json:"code"`
	Title                string                 `json:"title"`
	Description          string                 `json:"description"`
	Guidance             string                 `json:"guidance"`
	Category             string                 `json:"category,omitempty"`
	Objective            *string                `json:"objective,omitempty"`
	ControlType          *string                `json:"control_type,omitempty"`
	ImplementationType   *string                `json:"implementation_type,omitempty"`
	IsMandatory          bool                   `json:"is_mandatory"`
	Priority             ControlPriority        `json:"priority,omitempty"`
	SortOrder            int                    `json:"sort_order"`
	ParentControlID      *string                `json:"parent_control_id,omitempty"`
	DepthLevel           int                    `json:"depth_level"`
	EvidenceRequirements json.RawMessage        `json:"evidence_requirements"`
	TestProcedures       json.RawMessage        `json:"test_procedures"`
	References           json.RawMessage        `json:"references"`
	Keywords             []string               `json:"keywords"`
	Metadata             json.RawMessage        `json:"metadata"`
	Implementation       *ControlImplementation `json:"implementation,omitempty"`
}

type ControlImplementation struct {
	BaseModel
	OrganizationID            string                     `json:"organization_id"`
	FrameworkControlID        string                     `json:"framework_control_id"`
	OrganizationFrameworkID   string                     `json:"organization_framework_id"`
	Status                    ControlImplementationState `json:"status"`
	ImplementationStatus      ImplementationStatus       `json:"implementation_status"`
	MaturityLevel             int                        `json:"maturity_level"`
	OwnerUserID               *string                    `json:"owner_user_id,omitempty"`
	ReviewerUserID            *string                    `json:"reviewer_user_id,omitempty"`
	ImplementationDescription *string                    `json:"implementation_description,omitempty"`
	ImplementationNotes       *string                    `json:"implementation_notes,omitempty"`
	GapDescription            *string                    `json:"gap_description,omitempty"`
	RemediationPlan           *string                    `json:"remediation_plan,omitempty"`
	RemediationDueDate        *time.Time                 `json:"remediation_due_date,omitempty"`
	AutomationLevel           *string                    `json:"automation_level,omitempty"`
	Tags                      []string                   `json:"tags"`
	Metadata                  json.RawMessage            `json:"metadata"`
}

// ControlImplementationPatch contains only tenant-owned mutable fields.
type ControlImplementationPatch struct {
	Status                    *ControlImplementationState `json:"status,omitempty"`
	ImplementationStatus      *ImplementationStatus       `json:"implementation_status,omitempty"`
	MaturityLevel             *int                        `json:"maturity_level,omitempty"`
	OwnerUserID               *string                     `json:"owner_user_id,omitempty"`
	ReviewerUserID            *string                     `json:"reviewer_user_id,omitempty"`
	ImplementationDescription *string                     `json:"implementation_description,omitempty"`
	ImplementationNotes       *string                     `json:"implementation_notes,omitempty"`
	GapDescription            *string                     `json:"gap_description,omitempty"`
	RemediationPlan           *string                     `json:"remediation_plan,omitempty"`
	RemediationDueDate        *time.Time                  `json:"remediation_due_date,omitempty"`
	AutomationLevel           *string                     `json:"automation_level,omitempty"`
	Tags                      *[]string                   `json:"tags,omitempty"`
}

type ControlEvidence struct {
	BaseModel
	OrganizationID          string          `json:"organization_id"`
	ControlImplementationID string          `json:"control_implementation_id"`
	Title                   string          `json:"title"`
	Description             *string         `json:"description,omitempty"`
	EvidenceType            string          `json:"evidence_type"`
	FileName                *string         `json:"file_name,omitempty"`
	FileSizeBytes           *int64          `json:"file_size_bytes,omitempty"`
	MIMEType                *string         `json:"mime_type,omitempty"`
	FileHash                *string         `json:"file_hash,omitempty"`
	CollectionMethod        string          `json:"collection_method"`
	CollectedAt             time.Time       `json:"collected_at"`
	CollectedBy             *string         `json:"collected_by,omitempty"`
	ValidFrom               *time.Time      `json:"valid_from,omitempty"`
	ValidUntil              *time.Time      `json:"valid_until,omitempty"`
	IsCurrent               bool            `json:"is_current"`
	ReviewStatus            string          `json:"review_status"`
	Metadata                json.RawMessage `json:"metadata"`
}

// AttachControlEvidenceInput registers metadata for content handled by a
// separate upload/storage flow. It deliberately cannot set a server file path.
type AttachControlEvidenceInput struct {
	Title            string          `json:"title"`
	Description      *string         `json:"description,omitempty"`
	EvidenceType     string          `json:"evidence_type"`
	FileName         *string         `json:"file_name,omitempty"`
	FileSizeBytes    *int64          `json:"file_size_bytes,omitempty"`
	MIMEType         *string         `json:"mime_type,omitempty"`
	FileHash         *string         `json:"file_hash,omitempty"`
	CollectionMethod string          `json:"collection_method,omitempty"`
	ValidFrom        *time.Time      `json:"valid_from,omitempty"`
	ValidUntil       *time.Time      `json:"valid_until,omitempty"`
	Metadata         json.RawMessage `json:"metadata,omitempty"`
}

package models

import "time"

// AuditType classifies the nature of an audit engagement.
type AuditType string

const (
	AuditTypeInternal      AuditType = "Internal"
	AuditTypeExternal      AuditType = "External"
	AuditTypeCertification AuditType = "Certification"
)

// FindingStatus tracks the resolution state of an audit finding.
type FindingStatus string

const (
	FindingStatusOpen       FindingStatus = "Open"
	FindingStatusInProgress FindingStatus = "InProgress"
	FindingStatusResolved   FindingStatus = "Resolved"
	FindingStatusClosed     FindingStatus = "Closed"
	FindingStatusAccepted   FindingStatus = "Accepted"
)

// Audit represents a planned or completed audit engagement.
type Audit struct {
	TenantModel
	Title              string      `json:"title" gorm:"not null"`
	Description        string      `json:"description" gorm:"type:text"`
	Type               AuditType   `json:"type" gorm:"type:varchar(50);not null"`
	Status             AuditStatus `json:"status" gorm:"type:varchar(50);default:'Planned'"`
	LeadAuditorID      string      `json:"lead_auditor_id" gorm:"type:uuid"`
	Scope              string      `json:"scope" gorm:"type:text"`
	ScheduledStartDate *time.Time  `json:"scheduled_start_date,omitempty"`
	ScheduledEndDate   *time.Time  `json:"scheduled_end_date,omitempty"`
	ActualStartDate    *time.Time  `json:"actual_start_date,omitempty"`
	ActualEndDate      *time.Time  `json:"actual_end_date,omitempty"`
	FrameworkID        *string     `json:"framework_id,omitempty" gorm:"type:uuid"`
}

// AuditFinding represents a specific finding or observation from an audit.
type AuditFinding struct {
	TenantModel
	AuditID         string        `json:"audit_id" gorm:"type:uuid;not null;index"`
	ControlID       string        `json:"control_id" gorm:"type:uuid"`
	Title           string        `json:"title" gorm:"not null"`
	Description     string        `json:"description" gorm:"type:text"`
	Severity        string        `json:"severity" gorm:"type:varchar(50)"`
	Status          FindingStatus `json:"status" gorm:"type:varchar(50);default:'Open'"`
	RemediationPlan string        `json:"remediation_plan" gorm:"type:text"`
	OwnerID         string        `json:"owner_id" gorm:"type:uuid"`
	DueDate         *time.Time    `json:"due_date,omitempty"`
	ResolvedAt      *time.Time    `json:"resolved_at,omitempty"`
}

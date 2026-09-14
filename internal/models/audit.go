package models

import (
	"encoding/json"
	"time"
)

// AuditType classifies the nature of an audit engagement.
type AuditType string

const (
	AuditTypeInternal      AuditType = "internal"
	AuditTypeExternal      AuditType = "external"
	AuditTypeCertification AuditType = "certification"
)

// FindingStatus tracks the resolution state of an audit finding.
type FindingStatus string

const (
	FindingStatusOpen       FindingStatus = "open"
	FindingStatusInProgress FindingStatus = "in_progress"
	FindingStatusResolved   FindingStatus = "resolved"
	FindingStatusClosed     FindingStatus = "closed"
	FindingStatusAccepted   FindingStatus = "accepted"
)

// AuditPerson is the deliberately small user projection returned from audit
// endpoints. It avoids exposing authentication or account-security fields.
type AuditPerson struct {
	ID        string `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
}

// AuditFramework is the minimal framework projection needed by audit screens.
type AuditFramework struct {
	ID   string `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

// Audit represents a planned or completed audit engagement.
type Audit struct {
	TenantModel
	AuditRef           string          `json:"audit_ref"`
	Title              string          `json:"title" gorm:"not null"`
	Description        string          `json:"description" gorm:"type:text"`
	Type               AuditType       `json:"audit_type" gorm:"type:varchar(50);not null"`
	Status             AuditStatus     `json:"status" gorm:"type:varchar(50);default:'Planned'"`
	LeadAuditorID      string          `json:"lead_auditor_id" gorm:"type:uuid"`
	LeadAuditor        *AuditPerson    `json:"lead_auditor,omitempty"`
	Scope              string          `json:"scope" gorm:"type:text"`
	ScheduledStartDate *time.Time      `json:"scheduled_start_date,omitempty"`
	ScheduledEndDate   *time.Time      `json:"scheduled_end_date,omitempty"`
	ActualStartDate    *time.Time      `json:"actual_start_date,omitempty"`
	ActualEndDate      *time.Time      `json:"actual_end_date,omitempty"`
	FrameworkID        *string         `json:"framework_id,omitempty" gorm:"type:uuid"`
	Framework          *AuditFramework `json:"framework,omitempty"`
	CreatedBy          string          `json:"created_by"`
	Metadata           json.RawMessage `json:"metadata"`
	FindingsCount      int             `json:"findings_count"`
	CriticalOpen       int             `json:"critical_findings_open"`
	HighOpen           int             `json:"high_findings_open"`
}

// AuditFinding represents a specific finding or observation from an audit.
type AuditFinding struct {
	TenantModel
	AuditID            string          `json:"audit_id" gorm:"type:uuid;not null;index"`
	FindingRef         string          `json:"finding_ref"`
	ControlID          *string         `json:"control_id,omitempty" gorm:"type:uuid"`
	Title              string          `json:"title" gorm:"not null"`
	Description        string          `json:"description" gorm:"type:text"`
	Severity           string          `json:"severity" gorm:"type:varchar(50)"`
	Status             FindingStatus   `json:"status" gorm:"type:varchar(50);default:'open'"`
	FindingType        string          `json:"finding_type"`
	RootCause          string          `json:"root_cause"`
	Recommendation     string          `json:"recommendation"`
	RemediationPlan    string          `json:"remediation_plan" gorm:"type:text"`
	ResponsibleUserID  string          `json:"responsible_user_id" gorm:"type:uuid"`
	ResponsibleUser    *AuditPerson    `json:"responsible_user,omitempty"`
	DueDate            *time.Time      `json:"due_date,omitempty"`
	ResolvedAt         *time.Time      `json:"resolved_at,omitempty"`
	AcceptedRiskReason *string         `json:"accepted_risk_reason,omitempty"`
	CreatedBy          string          `json:"created_by"`
	Metadata           json.RawMessage `json:"metadata"`
}

type AuditCreateInput struct {
	Title              string          `json:"title"`
	Description        string          `json:"description"`
	AuditType          AuditType       `json:"audit_type"`
	LeadAuditorID      string          `json:"lead_auditor_id"`
	Scope              string          `json:"scope"`
	ScheduledStartDate string          `json:"scheduled_start_date"`
	ScheduledEndDate   string          `json:"scheduled_end_date"`
	FrameworkID        *string         `json:"framework_id,omitempty"`
	Metadata           json.RawMessage `json:"metadata,omitempty"`
}

type AuditPatch struct {
	Title              *string         `json:"title,omitempty"`
	Description        *string         `json:"description,omitempty"`
	AuditType          *AuditType      `json:"audit_type,omitempty"`
	LeadAuditorID      *string         `json:"lead_auditor_id,omitempty"`
	Scope              *string         `json:"scope,omitempty"`
	ScheduledStartDate *string         `json:"scheduled_start_date,omitempty"`
	ScheduledEndDate   *string         `json:"scheduled_end_date,omitempty"`
	FrameworkID        *string         `json:"framework_id,omitempty"`
	ClearFramework     bool            `json:"clear_framework,omitempty"`
	Metadata           json.RawMessage `json:"metadata,omitempty"`
}

type AuditListFilter struct {
	PaginationRequest
	Status        string
	AuditType     string
	LeadAuditorID string
	FrameworkID   string
	Search        string
}

type AuditFindingInput struct {
	ControlID         *string         `json:"control_id,omitempty"`
	Title             string          `json:"title"`
	Description       string          `json:"description"`
	Severity          string          `json:"severity"`
	FindingType       string          `json:"finding_type"`
	RootCause         string          `json:"root_cause,omitempty"`
	Recommendation    string          `json:"recommendation"`
	RemediationPlan   string          `json:"remediation_plan,omitempty"`
	ResponsibleUserID string          `json:"responsible_user_id"`
	DueDate           string          `json:"due_date"`
	Metadata          json.RawMessage `json:"metadata,omitempty"`
}

type AuditFindingPatch struct {
	ControlID          *string         `json:"control_id,omitempty"`
	ClearControl       bool            `json:"clear_control,omitempty"`
	Title              *string         `json:"title,omitempty"`
	Description        *string         `json:"description,omitempty"`
	Severity           *string         `json:"severity,omitempty"`
	Status             *FindingStatus  `json:"status,omitempty"`
	FindingType        *string         `json:"finding_type,omitempty"`
	RootCause          *string         `json:"root_cause,omitempty"`
	Recommendation     *string         `json:"recommendation,omitempty"`
	RemediationPlan    *string         `json:"remediation_plan,omitempty"`
	ResponsibleUserID  *string         `json:"responsible_user_id,omitempty"`
	DueDate            *string         `json:"due_date,omitempty"`
	AcceptedRiskReason *string         `json:"accepted_risk_reason,omitempty"`
	Metadata           json.RawMessage `json:"metadata,omitempty"`
}

type AuditFindingStats struct {
	Total        int `json:"total"`
	Open         int `json:"open"`
	InProgress   int `json:"in_progress"`
	Resolved     int `json:"resolved"`
	Closed       int `json:"closed"`
	Accepted     int `json:"accepted"`
	CriticalOpen int `json:"critical_open"`
	HighOpen     int `json:"high_open"`
	Overdue      int `json:"overdue"`
}

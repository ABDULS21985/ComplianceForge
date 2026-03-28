package models

import "time"

// BaseModel provides common fields for all database entities.
type BaseModel struct {
	ID        string     `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	CreatedAt time.Time  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt time.Time  `json:"updated_at" gorm:"autoUpdateTime"`
	DeletedAt *time.Time `json:"deleted_at,omitempty" gorm:"index"`
}

// TenantModel extends BaseModel with OrganizationID for multi-tenancy row-level security.
type TenantModel struct {
	BaseModel
	OrganizationID string `json:"organization_id" gorm:"type:uuid;not null;index"`
}

// PaginationRequest holds pagination parameters from the client.
type PaginationRequest struct {
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
}

// PaginationResponse returns pagination metadata to the client.
type PaginationResponse struct {
	Page       int `json:"page"`
	PageSize   int `json:"page_size"`
	TotalItems int `json:"total_items"`
	TotalPages int `json:"total_pages"`
}

// SortRequest holds sorting parameters from the client.
type SortRequest struct {
	Field     string `json:"field"`
	Direction string `json:"direction"` // "asc" or "desc"
}

// ErrorResponse is the standard API error payload.
type ErrorResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

// ComplianceStatus represents the compliance state of a control or requirement.
type ComplianceStatus string

const (
	ComplianceStatusCompliant          ComplianceStatus = "Compliant"
	ComplianceStatusNonCompliant       ComplianceStatus = "NonCompliant"
	ComplianceStatusPartiallyCompliant ComplianceStatus = "PartiallyCompliant"
	ComplianceStatusNotAssessed        ComplianceStatus = "NotAssessed"
)

// RiskLevel categorizes the severity of a risk.
type RiskLevel string

const (
	RiskLevelCritical RiskLevel = "Critical"
	RiskLevelHigh     RiskLevel = "High"
	RiskLevelMedium   RiskLevel = "Medium"
	RiskLevelLow      RiskLevel = "Low"
	RiskLevelVeryLow  RiskLevel = "VeryLow"
)

// PolicyStatus tracks the lifecycle state of a policy document.
type PolicyStatus string

const (
	PolicyStatusDraft       PolicyStatus = "Draft"
	PolicyStatusUnderReview PolicyStatus = "UnderReview"
	PolicyStatusApproved    PolicyStatus = "Approved"
	PolicyStatusPublished   PolicyStatus = "Published"
	PolicyStatusArchived    PolicyStatus = "Archived"
	PolicyStatusRetired     PolicyStatus = "Retired"
)

// AuditStatus tracks the progress of an audit engagement.
type AuditStatus string

const (
	AuditStatusPlanned    AuditStatus = "Planned"
	AuditStatusInProgress AuditStatus = "InProgress"
	AuditStatusCompleted  AuditStatus = "Completed"
	AuditStatusClosed     AuditStatus = "Closed"
)

// IncidentSeverity classifies the impact level of a security incident.
type IncidentSeverity string

const (
	IncidentSeverityCritical IncidentSeverity = "Critical"
	IncidentSeverityHigh     IncidentSeverity = "High"
	IncidentSeverityMedium   IncidentSeverity = "Medium"
	IncidentSeverityLow      IncidentSeverity = "Low"
)

// IncidentStatus tracks the resolution lifecycle of a security incident.
type IncidentStatus string

const (
	IncidentStatusOpen          IncidentStatus = "Open"
	IncidentStatusInvestigating IncidentStatus = "Investigating"
	IncidentStatusContained     IncidentStatus = "Contained"
	IncidentStatusResolved      IncidentStatus = "Resolved"
	IncidentStatusClosed        IncidentStatus = "Closed"
)

package models

import (
	"encoding/json"
	"time"
)

// ComplianceFramework is a framework catalog entry. OrganizationID is nil for
// platform-provided frameworks and set for private tenant frameworks.
type ComplianceFramework struct {
	BaseModel
	OrganizationID       *string                `json:"organization_id,omitempty"`
	Code                 string                 `json:"code"`
	Name                 string                 `json:"name"`
	FullName             *string                `json:"full_name,omitempty"`
	Version              string                 `json:"version"`
	Description          *string                `json:"description,omitempty"`
	IssuingBody          *string                `json:"issuing_body,omitempty"`
	Category             *string                `json:"category,omitempty"`
	ApplicableRegions    []string               `json:"applicable_regions"`
	ApplicableIndustries []string               `json:"applicable_industries"`
	IsSystemFramework    bool                   `json:"is_system_framework"`
	IsActive             bool                   `json:"is_active"`
	EffectiveDate        *time.Time             `json:"effective_date,omitempty"`
	SunsetDate           *time.Time             `json:"sunset_date,omitempty"`
	TotalControls        int                    `json:"total_controls"`
	IconURL              *string                `json:"icon_url,omitempty"`
	ColorHex             *string                `json:"color_hex,omitempty"`
	Metadata             json.RawMessage        `json:"metadata"`
	Adoption             *OrganizationFramework `json:"adoption,omitempty"`
}

// OrganizationFramework records a tenant's adoption of a catalog framework.
type OrganizationFramework struct {
	BaseModel
	OrganizationID       string     `json:"organization_id"`
	FrameworkID          string     `json:"framework_id"`
	Status               string     `json:"status"`
	AdoptionDate         *time.Time `json:"adoption_date,omitempty"`
	TargetCompletionDate *time.Time `json:"target_completion_date,omitempty"`
	ComplianceScore      float64    `json:"compliance_score"`
	ResponsibleUserID    *string    `json:"responsible_user_id,omitempty"`
}

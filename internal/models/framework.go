package models

import "time"

// ComplianceFramework represents a regulatory or standards framework (e.g. SOC 2, ISO 27001).
type ComplianceFramework struct {
	TenantModel
	Name          string     `json:"name" gorm:"not null"`
	Version       string     `json:"version"`
	Description   string     `json:"description" gorm:"type:text"`
	Authority     string     `json:"authority"`
	Category      string     `json:"category"`
	IsActive      bool       `json:"is_active" gorm:"default:true"`
	EffectiveDate *time.Time `json:"effective_date,omitempty"`

	// Controls is a convenience association; it is not stored on this table.
	Controls []Control `json:"controls,omitempty" gorm:"foreignKey:FrameworkID"`
}

package models

import "time"

// MitigationStatus tracks the treatment state of a risk.
type MitigationStatus string

const (
	MitigationStatusOpen        MitigationStatus = "Open"
	MitigationStatusInProgress  MitigationStatus = "InProgress"
	MitigationStatusMitigated   MitigationStatus = "Mitigated"
	MitigationStatusAccepted    MitigationStatus = "Accepted"
	MitigationStatusTransferred MitigationStatus = "Transferred"
)

// Risk represents an identified risk within the organization's risk register.
type Risk struct {
	TenantModel
	Title              string           `json:"title" gorm:"not null"`
	Description        string           `json:"description" gorm:"type:text"`
	Category           string           `json:"category"`
	Source             string           `json:"source"`
	Likelihood         int              `json:"likelihood" gorm:"check:likelihood >= 1 AND likelihood <= 5"`
	Impact             int              `json:"impact" gorm:"check:impact >= 1 AND impact <= 5"`
	RiskScore          float64          `json:"risk_score"`
	RiskLevel          RiskLevel        `json:"risk_level" gorm:"type:varchar(50)"`
	MitigationStrategy string           `json:"mitigation_strategy" gorm:"type:text"`
	MitigationStatus   MitigationStatus `json:"mitigation_status" gorm:"type:varchar(50);default:'Open'"`
	OwnerID            string           `json:"owner_id" gorm:"type:uuid"`
	RelatedControlID   *string          `json:"related_control_id,omitempty" gorm:"type:uuid"`
	ReviewDate         *time.Time       `json:"review_date,omitempty"`
}

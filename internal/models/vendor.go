package models

import "time"

// VendorStatus tracks the lifecycle state of a third-party vendor relationship.
type VendorStatus string

const (
	VendorStatusActive      VendorStatus = "Active"
	VendorStatusUnderReview VendorStatus = "UnderReview"
	VendorStatusApproved    VendorStatus = "Approved"
	VendorStatusSuspended   VendorStatus = "Suspended"
	VendorStatusTerminated  VendorStatus = "Terminated"
)

// Vendor represents a third-party vendor subject to risk assessment and ongoing monitoring.
type Vendor struct {
	TenantModel
	Name                       string       `json:"name" gorm:"not null"`
	Description                string       `json:"description" gorm:"type:text"`
	ContactName                string       `json:"contact_name"`
	ContactEmail               string       `json:"contact_email"`
	ContactPhone               string       `json:"contact_phone"`
	Website                    string       `json:"website"`
	Category                   string       `json:"category"`
	RiskLevel                  RiskLevel    `json:"risk_level" gorm:"type:varchar(50)"`
	Status                     VendorStatus `json:"status" gorm:"type:varchar(50);default:'Active'"`
	ContractStartDate          *time.Time   `json:"contract_start_date,omitempty"`
	ContractEndDate            *time.Time   `json:"contract_end_date,omitempty"`
	LastAssessmentDate         *time.Time   `json:"last_assessment_date,omitempty"`
	NextAssessmentDate         *time.Time   `json:"next_assessment_date,omitempty"`
	DataProcessingAgreement    bool         `json:"data_processing_agreement" gorm:"default:false"`
	SubProcessors              []string     `json:"sub_processors" gorm:"type:jsonb;serializer:json"`
}

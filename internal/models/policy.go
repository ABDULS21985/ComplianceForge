package models

import "time"

// Policy represents a governance policy document subject to review and approval workflows.
type Policy struct {
	TenantModel
	Title              string       `json:"title" gorm:"not null"`
	Version            string       `json:"version"`
	Content            string       `json:"content" gorm:"type:text"`
	Category           string       `json:"category"`
	Status             PolicyStatus `json:"status" gorm:"type:varchar(50);default:'Draft'"`
	OwnerID            string       `json:"owner_id" gorm:"type:uuid"`
	ApproverID         *string      `json:"approver_id,omitempty" gorm:"type:uuid"`
	ApprovedAt         *time.Time   `json:"approved_at,omitempty"`
	EffectiveDate      *time.Time   `json:"effective_date,omitempty"`
	ReviewDate         *time.Time   `json:"review_date,omitempty"`
	RelatedFrameworkID *string      `json:"related_framework_id,omitempty" gorm:"type:uuid"`
}

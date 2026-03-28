package models

// ControlPriority indicates the urgency of implementing a control.
type ControlPriority string

const (
	ControlPriorityCritical ControlPriority = "Critical"
	ControlPriorityHigh     ControlPriority = "High"
	ControlPriorityMedium   ControlPriority = "Medium"
	ControlPriorityLow      ControlPriority = "Low"
)

// ImplementationStatus tracks the implementation state of a control.
type ImplementationStatus string

const (
	ImplementationStatusNotStarted    ImplementationStatus = "NotStarted"
	ImplementationStatusInProgress    ImplementationStatus = "InProgress"
	ImplementationStatusImplemented   ImplementationStatus = "Implemented"
	ImplementationStatusNotApplicable ImplementationStatus = "NotApplicable"
)

// Control represents a specific security or compliance control within a framework.
type Control struct {
	TenantModel
	FrameworkID          string               `json:"framework_id" gorm:"type:uuid;not null;index"`
	Code                 string               `json:"code" gorm:"not null"`
	Title                string               `json:"title" gorm:"not null"`
	Description          string               `json:"description" gorm:"type:text"`
	Category             string               `json:"category"`
	Guidance             string               `json:"guidance" gorm:"type:text"`
	ComplianceStatus     ComplianceStatus     `json:"compliance_status" gorm:"type:varchar(50);default:'NotAssessed'"`
	Priority             ControlPriority      `json:"priority" gorm:"type:varchar(50)"`
	ImplementationStatus ImplementationStatus `json:"implementation_status" gorm:"type:varchar(50);default:'NotStarted'"`
	OwnerID              string               `json:"owner_id" gorm:"type:uuid"`
	EvidenceRequired     string               `json:"evidence_required" gorm:"type:text"`
}

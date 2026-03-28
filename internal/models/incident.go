package models

import "time"

// Incident represents a security or compliance incident requiring investigation and response.
type Incident struct {
	TenantModel
	Title                string           `json:"title" gorm:"not null"`
	Description          string           `json:"description" gorm:"type:text"`
	Severity             IncidentSeverity `json:"severity" gorm:"type:varchar(50);not null"`
	Status               IncidentStatus   `json:"status" gorm:"type:varchar(50);default:'Open'"`
	Category             string           `json:"category"`
	ReporterID           string           `json:"reporter_id" gorm:"type:uuid;not null"`
	AssigneeID           string           `json:"assignee_id" gorm:"type:uuid"`
	DetectedAt           *time.Time       `json:"detected_at,omitempty"`
	ContainedAt          *time.Time       `json:"contained_at,omitempty"`
	ResolvedAt           *time.Time       `json:"resolved_at,omitempty"`
	RootCause            string           `json:"root_cause" gorm:"type:text"`
	Impact               string           `json:"impact" gorm:"type:text"`
	LessonsLearned       string           `json:"lessons_learned" gorm:"type:text"`
	IsBreachNotifiable   bool             `json:"is_breach_notifiable" gorm:"default:false"`
	NotificationDeadline *time.Time       `json:"notification_deadline,omitempty"` // GDPR 72-hour notification window
	RelatedAssetID       *string          `json:"related_asset_id,omitempty" gorm:"type:uuid"`
}

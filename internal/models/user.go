package models

import "time"

// UserRole defines the access level of a user within an organization.
type UserRole string

const (
	UserRoleAdmin             UserRole = "Admin"
	UserRoleAuditor           UserRole = "Auditor"
	UserRoleComplianceOfficer UserRole = "ComplianceOfficer"
	UserRoleRiskManager       UserRole = "RiskManager"
	UserRoleViewer            UserRole = "Viewer"
)

// User represents an authenticated user belonging to an organization.
type User struct {
	TenantModel
	Email        string     `json:"email" gorm:"uniqueIndex;not null"`
	PasswordHash string     `json:"-" gorm:"not null"`
	FirstName    string     `json:"first_name"`
	LastName     string     `json:"last_name"`
	Role         UserRole   `json:"role" gorm:"type:varchar(50);not null"`
	Department   string     `json:"department"`
	Phone        string     `json:"phone"`
	IsActive     bool       `json:"is_active" gorm:"default:true"`
	LastLoginAt  *time.Time `json:"last_login_at,omitempty"`
	MFAEnabled   bool       `json:"mfa_enabled" gorm:"default:false"`
}

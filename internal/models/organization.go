package models

// Organization represents a tenant in the multi-tenant GRC platform.
// All other entities reference an Organization via OrganizationID for RLS.
type Organization struct {
	BaseModel
	Name             string            `json:"name" gorm:"not null"`
	Domain           string            `json:"domain" gorm:"uniqueIndex;not null"`
	Industry         string            `json:"industry"`
	Country          string            `json:"country"`
	Timezone         string            `json:"timezone"`
	SubscriptionTier string            `json:"subscription_tier"`
	LogoURL          string            `json:"logo_url"`
	IsActive         bool              `json:"is_active" gorm:"default:true"`
	Settings         map[string]any    `json:"settings" gorm:"type:jsonb;serializer:json"`
	MaxUsers         int               `json:"max_users"`
}

package models

// Organization represents a tenant in the multi-tenant GRC platform.
// All other entities reference an Organization via OrganizationID for RLS.
type Organization struct {
	BaseModel
	Name                 string         `json:"name"`
	Slug                 string         `json:"slug"`
	LegalName            string         `json:"legal_name,omitempty"`
	RegistrationNumber   string         `json:"registration_number,omitempty"`
	TaxID                string         `json:"tax_id,omitempty"`
	Industry             string         `json:"industry,omitempty"`
	Sector               string         `json:"sector,omitempty"`
	CountryCode          string         `json:"country_code,omitempty"`
	HeadquartersAddress  map[string]any `json:"headquarters_address"`
	Status               string         `json:"status"`
	Tier                 string         `json:"tier"`
	Settings             map[string]any `json:"settings"`
	Branding             map[string]any `json:"branding"`
	Timezone             string         `json:"timezone"`
	DefaultLanguage      string         `json:"default_language"`
	SupportedLanguages   []string       `json:"supported_languages"`
	EmployeeCountRange   string         `json:"employee_count_range,omitempty"`
	AnnualRevenueRange   string         `json:"annual_revenue_range,omitempty"`
	ParentOrganizationID *string        `json:"parent_organization_id,omitempty"`
	Metadata             map[string]any `json:"metadata"`
}

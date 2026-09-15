package models

import (
	"errors"
	"time"
)

const OrganizationProfileMaximumVersion int64 = (1 << 53) - 1
const OrganizationProfileScope = "tenant_organization_profile"

var (
	ErrOrganizationProfileScope       = errors.New("organization profile tenant or actor context is invalid")
	ErrOrganizationProfileNotFound    = errors.New("organization profile is unavailable for this tenant")
	ErrOrganizationProfileConflict    = errors.New("organization profile changed; reload before saving")
	ErrOrganizationProfileUnavailable = errors.New("organization profile is temporarily unavailable")
)

// OrganizationContact is informational, not an email transport configuration,
// billing instruction, verified domain, or grant of administrator authority.
type OrganizationContact struct {
	Purpose string `json:"purpose"`
	Email   string `json:"email"`
}

// OrganizationProfile projects only reviewed profile fields. It never embeds
// raw settings, branding, metadata, credentials, or parent-tenant references.
type OrganizationProfile struct {
	ID                   string                `json:"id"`
	Name                 string                `json:"name"`
	Slug                 string                `json:"slug"`
	LegalName            string                `json:"legal_name"`
	Industry             string                `json:"industry"`
	CountryCode          string                `json:"country_code"`
	Timezone             string                `json:"timezone"`
	DefaultLanguage      string                `json:"default_language"`
	SupportedLanguages   []string              `json:"supported_languages"`
	EmployeeCountRange   string                `json:"employee_count_range"`
	Status               string                `json:"status"`
	Tier                 string                `json:"tier"`
	FiscalYearStartMonth int                   `json:"fiscal_year_start_month"`
	FiscalYearStartDay   int                   `json:"fiscal_year_start_day"`
	Contacts             []OrganizationContact `json:"contacts"`
	Version              int64                 `json:"version"`
	UpdatedAt            time.Time             `json:"updated_at"`
}

// Editable proves that this projection was not masked/hidden. It is not a
// settings:configure grant: the mutation policy is evaluated independently.
type OrganizationProfileMeta struct {
	SchemaVersion int    `json:"schema_version"`
	Scope         string `json:"scope"`
	Editable      bool   `json:"editable"`
}

type OrganizationProfileResponse struct {
	Data *OrganizationProfile    `json:"data"`
	Meta OrganizationProfileMeta `json:"meta"`
}

// OrganizationProfileUpdateInput uses presence-aware fields for the legacy PUT
// URL: absent fields are preserved, explicit null is rejected by the handler.
// Read-only identifiers, lifecycle, tier, raw settings and domains are not inputs.
type OrganizationProfileUpdateInput struct {
	ExpectedVersion      int64                  `json:"expected_version"`
	Reason               string                 `json:"reason"`
	Name                 *string                `json:"name,omitempty"`
	LegalName            *string                `json:"legal_name,omitempty"`
	Industry             *string                `json:"industry,omitempty"`
	CountryCode          *string                `json:"country_code,omitempty"`
	Timezone             *string                `json:"timezone,omitempty"`
	DefaultLanguage      *string                `json:"default_language,omitempty"`
	SupportedLanguages   *[]string              `json:"supported_languages,omitempty"`
	EmployeeCountRange   *string                `json:"employee_count_range,omitempty"`
	FiscalYearStartMonth *int                   `json:"fiscal_year_start_month,omitempty"`
	FiscalYearStartDay   *int                   `json:"fiscal_year_start_day,omitempty"`
	Contacts             *[]OrganizationContact `json:"contacts,omitempty"`
}

// OrganizationProfileValidationError exposes only reviewed field/code labels,
// never the rejected value, mail parser detail, or a provider/database error.
type OrganizationProfileValidationError struct {
	Field string `json:"field"`
	Code  string `json:"code"`
}

func (e *OrganizationProfileValidationError) Error() string {
	return "organization profile field is invalid"
}

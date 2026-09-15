package service

import (
	"context"
	"errors"
	"net/mail"
	"slices"
	"strings"
	"time"
	_ "time/tzdata" // Carry IANA data into minimal production images.
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/complianceforge/platform/internal/models"
)

type OrganizationProfileStore interface {
	GetOrganizationProfile(context.Context, string, string) (*models.OrganizationProfile, error)
	UpdateOrganizationProfile(context.Context, string, string, string, models.OrganizationProfile, string) (*models.OrganizationProfile, error)
}

type OrganizationProfileService struct{ store OrganizationProfileStore }

func NewOrganizationProfileService(store OrganizationProfileStore) (*OrganizationProfileService, error) {
	if interfaceValueIsNil(store) {
		return nil, errors.New("organization profile store is required")
	}
	return &OrganizationProfileService{store: store}, nil
}

func (s *OrganizationProfileService) GetProfile(ctx context.Context, orgID, actorID string) (*models.OrganizationProfile, error) {
	if !validOrganizationProfileID(orgID) || !validOrganizationProfileID(actorID) {
		return nil, models.ErrOrganizationProfileScope
	}
	if ctx.Err() != nil {
		return nil, models.ErrOrganizationProfileUnavailable
	}
	profile, err := s.store.GetOrganizationProfile(ctx, orgID, actorID)
	if err != nil {
		return nil, safeOrganizationProfileError(err)
	}
	if profile == nil || profile.ID != orgID || profile.Version < 1 || profile.Version > models.OrganizationProfileMaximumVersion || profile.UpdatedAt.IsZero() {
		return nil, models.ErrOrganizationProfileUnavailable
	}
	copy := *profile
	copy.Contacts = slices.Clone(profile.Contacts)
	copy.SupportedLanguages = slices.Clone(profile.SupportedLanguages)
	profile = &copy
	// Legacy profile strings remain readable for corrective administration, but
	// the reserved typed contact/fiscal fields must obey their v59 contract.
	if profile.Contacts == nil {
		profile.Contacts = []models.OrganizationContact{}
	}
	if profile.SupportedLanguages == nil {
		profile.SupportedLanguages = []string{}
	}
	if validateOrganizationContacts(profile.Contacts) != nil || !validFiscalStart(profile.FiscalYearStartMonth, profile.FiscalYearStartDay) {
		return nil, models.ErrOrganizationProfileUnavailable
	}
	return profile, nil
}

func (s *OrganizationProfileService) UpdateProfile(
	ctx context.Context, orgID, actorID, requestID string, input models.OrganizationProfileUpdateInput,
) (*models.OrganizationProfile, error) {
	if !validOrganizationProfileID(orgID) || !validOrganizationProfileID(actorID) ||
		(requestID != "" && !validOrganizationProfileID(requestID)) {
		return nil, models.ErrOrganizationProfileScope
	}
	if input.ExpectedVersion < 1 || input.ExpectedVersion > models.OrganizationProfileMaximumVersion {
		return nil, profileValidation("expected_version", "positive_version_required")
	}
	if !validProfileText(input.Reason, 500, true) {
		return nil, profileValidation("reason", "bounded_reason_required")
	}
	if !organizationProfileHasChanges(input) {
		return nil, profileValidation("profile", "editable_field_required")
	}
	current, err := s.GetProfile(ctx, orgID, actorID)
	if err != nil {
		return nil, err
	}
	if current.Version != input.ExpectedVersion {
		return nil, models.ErrOrganizationProfileConflict
	}
	if current.Version == models.OrganizationProfileMaximumVersion {
		return nil, models.ErrOrganizationProfileUnavailable
	}
	updated := *current
	updated.Contacts = slices.Clone(current.Contacts)
	updated.SupportedLanguages = slices.Clone(current.SupportedLanguages)
	for _, field := range []struct {
		name     string
		input    *string
		target   *string
		limit    int
		required bool
	}{
		{"name", input.Name, &updated.Name, 255, true},
		{"legal_name", input.LegalName, &updated.LegalName, 500, false},
		{"industry", input.Industry, &updated.Industry, 100, false},
		{"employee_count_range", input.EmployeeCountRange, &updated.EmployeeCountRange, 20, false},
	} {
		if field.input != nil {
			if !validProfileText(*field.input, field.limit, field.required) {
				return nil, profileValidation(field.name, "invalid_bounded_text")
			}
			*field.target = strings.TrimSpace(*field.input)
		}
	}
	if input.CountryCode != nil {
		country := *input.CountryCode
		if country != "" && (len(country) != 2 || country[0] < 'A' || country[0] > 'Z' || country[1] < 'A' || country[1] > 'Z') {
			return nil, profileValidation("country_code", "uppercase_two_letter_code_required")
		}
		updated.CountryCode = country
	}
	if input.Timezone != nil {
		zone := *input.Timezone
		if !validProfileTimezone(zone) {
			return nil, profileValidation("timezone", "iana_timezone_required")
		}
		updated.Timezone = zone
	}
	if input.DefaultLanguage != nil {
		if !supportedProfileLanguage(*input.DefaultLanguage) {
			return nil, profileValidation("default_language", "unsupported_language")
		}
		updated.DefaultLanguage = *input.DefaultLanguage
	}
	if input.SupportedLanguages != nil {
		languages := slices.Clone(*input.SupportedLanguages)
		if len(languages) < 1 || len(languages) > 3 {
			return nil, profileValidation("supported_languages", "one_to_three_languages_required")
		}
		slices.Sort(languages)
		for index, language := range languages {
			if !supportedProfileLanguage(language) || (index > 0 && languages[index-1] == language) {
				return nil, profileValidation("supported_languages", "unique_supported_languages_required")
			}
		}
		updated.SupportedLanguages = languages
	}
	if input.DefaultLanguage != nil || input.SupportedLanguages != nil {
		if !slices.Contains(updated.SupportedLanguages, updated.DefaultLanguage) {
			return nil, profileValidation("supported_languages", "default_language_must_be_included")
		}
	}
	if input.FiscalYearStartMonth != nil {
		updated.FiscalYearStartMonth = *input.FiscalYearStartMonth
	}
	if input.FiscalYearStartDay != nil {
		updated.FiscalYearStartDay = *input.FiscalYearStartDay
	}
	if !validFiscalStart(updated.FiscalYearStartMonth, updated.FiscalYearStartDay) {
		return nil, profileValidation("fiscal_year_start", "valid_non_leap_recurring_date_required")
	}
	if input.Contacts != nil {
		if err := validateOrganizationContacts(*input.Contacts); err != nil {
			return nil, err
		}
		updated.Contacts = slices.Clone(*input.Contacts)
		if updated.Contacts == nil {
			updated.Contacts = []models.OrganizationContact{}
		}
		slices.SortFunc(updated.Contacts, func(a, b models.OrganizationContact) int { return strings.Compare(a.Purpose, b.Purpose) })
	}
	result, err := s.store.UpdateOrganizationProfile(ctx, orgID, actorID, requestID, updated, strings.TrimSpace(input.Reason))
	if err != nil {
		return nil, safeOrganizationProfileError(err)
	}
	if result == nil || result.ID != orgID || result.Version != current.Version+1 || result.UpdatedAt.IsZero() {
		return nil, models.ErrOrganizationProfileUnavailable
	}
	return result, nil
}

func validOrganizationProfileID(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id != uuid.Nil && id.String() == value
}

func profileValidation(field, code string) error {
	return &models.OrganizationProfileValidationError{Field: field, Code: code}
}

func validProfileText(value string, maximum int, required bool) bool {
	if !utf8.ValidString(value) || len(value) > maximum || (required && strings.TrimSpace(value) == "") {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func validProfileTimezone(zone string) bool {
	if len(zone) < 1 || len(zone) > 50 || zone == "Local" || strings.TrimSpace(zone) != zone {
		return false
	}
	for _, character := range zone {
		if !(character >= 'A' && character <= 'Z') && !(character >= 'a' && character <= 'z') &&
			!(character >= '0' && character <= '9') && character != '/' && character != '_' && character != '-' && character != '+' {
			return false
		}
	}
	_, err := time.LoadLocation(zone)
	return err == nil
}

func supportedProfileLanguage(language string) bool {
	return language == "en" || language == "de" || language == "fr"
}

func validFiscalStart(month, day int) bool {
	if month < 1 || month > 12 || day < 1 || day > 31 {
		return false
	}
	date := time.Date(2001, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	return int(date.Month()) == month && date.Day() == day
}

func validateOrganizationContacts(contacts []models.OrganizationContact) error {
	if len(contacts) > 4 {
		return profileValidation("contacts", "at_most_four_contacts")
	}
	seen := map[string]bool{}
	for _, contact := range contacts {
		if !slices.Contains([]string{"security", "privacy", "billing", "support"}, contact.Purpose) || seen[contact.Purpose] {
			return profileValidation("contacts", "unique_reviewed_purpose_required")
		}
		if len(contact.Email) < 3 || len(contact.Email) > 254 {
			return profileValidation("contacts", "plain_ascii_email_required")
		}
		for _, character := range contact.Email {
			if character < 33 || character > 126 {
				return profileValidation("contacts", "plain_ascii_email_required")
			}
		}
		address, err := mail.ParseAddress(contact.Email)
		if err != nil || address.Name != "" || address.Address != contact.Email {
			return profileValidation("contacts", "plain_ascii_email_required")
		}
		seen[contact.Purpose] = true
	}
	return nil
}

func organizationProfileHasChanges(input models.OrganizationProfileUpdateInput) bool {
	return input.Name != nil || input.LegalName != nil || input.Industry != nil || input.CountryCode != nil ||
		input.Timezone != nil || input.DefaultLanguage != nil || input.SupportedLanguages != nil || input.EmployeeCountRange != nil ||
		input.FiscalYearStartMonth != nil || input.FiscalYearStartDay != nil || input.Contacts != nil
}

func safeOrganizationProfileError(err error) error {
	for _, safe := range []error{models.ErrOrganizationProfileScope, models.ErrOrganizationProfileNotFound, models.ErrOrganizationProfileConflict} {
		if errors.Is(err, safe) {
			return safe
		}
	}
	return models.ErrOrganizationProfileUnavailable
}

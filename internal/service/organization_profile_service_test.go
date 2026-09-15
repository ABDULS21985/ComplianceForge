package service

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/complianceforge/platform/internal/models"
)

const profileServiceOrg = "20000000-aaaa-4000-8000-000000000001"
const profileServiceActor = "30000000-aaaa-4000-8000-000000000001"

type organizationProfileStoreStub struct {
	profile               *models.OrganizationProfile
	err                   error
	getCalls, updateCalls int
	updated               models.OrganizationProfile
	reason, request       string
}

func (s *organizationProfileStoreStub) GetOrganizationProfile(context.Context, string, string) (*models.OrganizationProfile, error) {
	s.getCalls++
	return s.profile, s.err
}

func (s *organizationProfileStoreStub) UpdateOrganizationProfile(_ context.Context, _, _, request string, profile models.OrganizationProfile, reason string) (*models.OrganizationProfile, error) {
	s.updateCalls++
	s.updated, s.reason, s.request = profile, reason, request
	if s.err != nil {
		return nil, s.err
	}
	profile.Version++
	return &profile, nil
}

func organizationProfileServiceFixture() *models.OrganizationProfile {
	return &models.OrganizationProfile{
		ID: profileServiceOrg, Name: "Tenant", Slug: "tenant", LegalName: "Tenant Legal",
		Industry: "Services", CountryCode: "GB", Timezone: "Europe/London",
		DefaultLanguage: "en", SupportedLanguages: []string{"en"}, Status: "active", Tier: "enterprise",
		FiscalYearStartMonth: 1, FiscalYearStartDay: 1, Contacts: []models.OrganizationContact{},
		Version: 1, UpdatedAt: time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC),
	}
}

func profileString(value string) *string { return &value }
func profileInt(value int) *int          { return &value }

func TestOrganizationProfileServiceRequiresStoreAndOwnsProjection(t *testing.T) {
	var typedNil *organizationProfileStoreStub
	for _, store := range []OrganizationProfileStore{nil, typedNil} {
		if _, err := NewOrganizationProfileService(store); err == nil {
			t.Fatal("missing store accepted")
		}
	}
	fixture := organizationProfileServiceFixture()
	fixture.Contacts = []models.OrganizationContact{{Purpose: "security", Email: "security@example.com"}}
	store := &organizationProfileStoreStub{profile: fixture}
	svc, _ := NewOrganizationProfileService(store)
	result, err := svc.GetProfile(context.Background(), profileServiceOrg, profileServiceActor)
	if err != nil {
		t.Fatal(err)
	}
	result.Contacts[0].Email = "changed@example.com"
	result.SupportedLanguages[0] = "fr"
	if fixture.Contacts[0].Email != "security@example.com" || fixture.SupportedLanguages[0] != "en" {
		t.Fatal("caller mutated shared profile projection")
	}
}

func TestOrganizationProfileServiceRejectsIdentityAliasesBeforeStore(t *testing.T) {
	for _, value := range []string{"", "00000000-0000-0000-0000-000000000000", strings.ToUpper(profileServiceOrg), "urn:uuid:" + profileServiceOrg, "{" + profileServiceOrg + "}", strings.ReplaceAll(profileServiceOrg, "-", "")} {
		for _, field := range []string{"organization", "actor", "request"} {
			if field == "request" && value == "" {
				continue
			}
			t.Run(field+value, func(t *testing.T) {
				store := &organizationProfileStoreStub{profile: organizationProfileServiceFixture()}
				svc, _ := NewOrganizationProfileService(store)
				org, actor, request := profileServiceOrg, profileServiceActor, ""
				switch field {
				case "organization":
					org = value
				case "actor":
					actor = value
				case "request":
					request = value
				}
				_, err := svc.UpdateProfile(context.Background(), org, actor, request, models.OrganizationProfileUpdateInput{ExpectedVersion: 1, Reason: "Reviewed", Name: profileString("New")})
				if !errors.Is(err, models.ErrOrganizationProfileScope) || store.getCalls != 0 || store.updateCalls != 0 {
					t.Fatalf("identity alias reached store: %v", err)
				}
			})
		}
	}
}

func TestOrganizationProfileServicePreservesAbsentFieldsAndCanonicalizesChoices(t *testing.T) {
	store := &organizationProfileStoreStub{profile: organizationProfileServiceFixture()}
	svc, _ := NewOrganizationProfileService(store)
	languages := []string{"fr", "en"}
	contacts := []models.OrganizationContact{{Purpose: "security", Email: "security@example.com"}, {Purpose: "privacy", Email: "privacy@example.com"}}
	input := models.OrganizationProfileUpdateInput{
		ExpectedVersion: 1, Reason: "  Approved profile review  ", Name: profileString("  Revised Tenant  "),
		Timezone: profileString("Africa/Lagos"), DefaultLanguage: profileString("fr"), SupportedLanguages: &languages,
		FiscalYearStartMonth: profileInt(4), FiscalYearStartDay: profileInt(1), Contacts: &contacts,
	}
	result, err := svc.UpdateProfile(context.Background(), profileServiceOrg, profileServiceActor, "", input)
	if err != nil {
		t.Fatal(err)
	}
	if result.Version != 2 || result.Name != "Revised Tenant" || result.LegalName != "Tenant Legal" || result.Slug != "tenant" || result.Tier != "enterprise" || result.Status != "active" ||
		store.reason != "Approved profile review" || store.updated.SupportedLanguages[0] != "en" || store.updated.Contacts[0].Purpose != "privacy" || store.updateCalls != 1 {
		t.Fatalf("wrong safe merge: %+v", result)
	}
	if languages[0] != "fr" || contacts[0].Purpose != "security" || store.profile.Name != "Tenant" {
		t.Fatal("input/store changed during merge")
	}
}

func TestOrganizationProfileServiceValidatesBoundedFields(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*models.OrganizationProfileUpdateInput)
	}{
		{"version", func(i *models.OrganizationProfileUpdateInput) { i.ExpectedVersion = 0 }},
		{"negative version", func(i *models.OrganizationProfileUpdateInput) { i.ExpectedVersion = -1 }},
		{"unsafe JavaScript version", func(i *models.OrganizationProfileUpdateInput) { i.ExpectedVersion = math.MaxInt64 }},
		{"reason", func(i *models.OrganizationProfileUpdateInput) { i.Reason = " " }},
		{"long reason", func(i *models.OrganizationProfileUpdateInput) { i.Reason = strings.Repeat("a", 501) }},
		{"reason controls", func(i *models.OrganizationProfileUpdateInput) { i.Reason = "review\nsecret" }},
		{"no editable field", func(i *models.OrganizationProfileUpdateInput) { i.Name = nil }},
		{"empty name", func(i *models.OrganizationProfileUpdateInput) { i.Name = profileString(" ") }},
		{"invalid utf8", func(i *models.OrganizationProfileUpdateInput) { i.Name = profileString(string([]byte{255})) }},
		{"long name", func(i *models.OrganizationProfileUpdateInput) { i.Name = profileString(strings.Repeat("a", 256)) }},
		{"country lowercase", func(i *models.OrganizationProfileUpdateInput) { i.CountryCode = profileString("gb") }},
		{"country long", func(i *models.OrganizationProfileUpdateInput) { i.CountryCode = profileString("GBR") }},
		{"local timezone", func(i *models.OrganizationProfileUpdateInput) { i.Timezone = profileString("Local") }},
		{"path timezone", func(i *models.OrganizationProfileUpdateInput) { i.Timezone = profileString("../secret") }},
		{"invalid timezone", func(i *models.OrganizationProfileUpdateInput) { i.Timezone = profileString("Unknown/Zone") }},
		{"timezone controls", func(i *models.OrganizationProfileUpdateInput) { i.Timezone = profileString("UTC\n") }},
		{"unsupported language", func(i *models.OrganizationProfileUpdateInput) { i.DefaultLanguage = profileString("es") }},
		{"unlisted default", func(i *models.OrganizationProfileUpdateInput) { i.DefaultLanguage = profileString("fr") }},
		{"duplicate language", func(i *models.OrganizationProfileUpdateInput) { v := []string{"en", "en"}; i.SupportedLanguages = &v }},
		{"empty languages", func(i *models.OrganizationProfileUpdateInput) { v := []string{}; i.SupportedLanguages = &v }},
		{"too many languages", func(i *models.OrganizationProfileUpdateInput) {
			v := []string{"en", "de", "fr", "es"}
			i.SupportedLanguages = &v
		}},
		{"fiscal zero", func(i *models.OrganizationProfileUpdateInput) { i.FiscalYearStartMonth = profileInt(0) }},
		{"April31", func(i *models.OrganizationProfileUpdateInput) {
			i.FiscalYearStartMonth = profileInt(4)
			i.FiscalYearStartDay = profileInt(31)
		}},
		{"Feb29", func(i *models.OrganizationProfileUpdateInput) {
			i.FiscalYearStartMonth = profileInt(2)
			i.FiscalYearStartDay = profileInt(29)
		}},
		{"contact purpose", func(i *models.OrganizationProfileUpdateInput) {
			v := []models.OrganizationContact{{Purpose: "root", Email: "a@example.com"}}
			i.Contacts = &v
		}},
		{"contact duplicate", func(i *models.OrganizationProfileUpdateInput) {
			v := []models.OrganizationContact{{Purpose: "security", Email: "a@example.com"}, {Purpose: "security", Email: "b@example.com"}}
			i.Contacts = &v
		}},
		{"display address", func(i *models.OrganizationProfileUpdateInput) {
			v := []models.OrganizationContact{{Purpose: "security", Email: "Person <a@example.com>"}}
			i.Contacts = &v
		}},
		{"address controls", func(i *models.OrganizationProfileUpdateInput) {
			v := []models.OrganizationContact{{Purpose: "security", Email: "a@example.com\n"}}
			i.Contacts = &v
		}},
		{"address unicode", func(i *models.OrganizationProfileUpdateInput) {
			v := []models.OrganizationContact{{Purpose: "security", Email: "é@example.com"}}
			i.Contacts = &v
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &organizationProfileStoreStub{profile: organizationProfileServiceFixture()}
			svc, _ := NewOrganizationProfileService(store)
			input := models.OrganizationProfileUpdateInput{ExpectedVersion: 1, Reason: "Reviewed", Name: profileString("New")}
			test.change(&input)
			_, err := svc.UpdateProfile(context.Background(), profileServiceOrg, profileServiceActor, "", input)
			var validation *models.OrganizationProfileValidationError
			if !errors.As(err, &validation) || store.updateCalls != 0 {
				t.Fatalf("invalid field saved: %v", err)
			}
			if strings.Contains(err.Error(), "secret") {
				t.Fatal("validation leaked rejected value")
			}
		})
	}
}

func TestOrganizationProfileServiceFailsSafelyOnStoreAndVersionErrors(t *testing.T) {
	for _, version := range []int64{2, models.OrganizationProfileMaximumVersion} {
		profile := organizationProfileServiceFixture()
		profile.Version = version
		store := &organizationProfileStoreStub{profile: profile}
		svc, _ := NewOrganizationProfileService(store)
		expected := int64(1)
		want := models.ErrOrganizationProfileConflict
		if version == models.OrganizationProfileMaximumVersion {
			expected = version
			want = models.ErrOrganizationProfileUnavailable
		}
		_, err := svc.UpdateProfile(context.Background(), profileServiceOrg, profileServiceActor, "", models.OrganizationProfileUpdateInput{ExpectedVersion: expected, Reason: "Reviewed", Name: profileString("New")})
		if !errors.Is(err, want) || store.updateCalls != 0 {
			t.Fatalf("invalid version saved: %v", err)
		}
	}
	store := &organizationProfileStoreStub{err: errors.New("postgres://secret-user:secret-password@internal")}
	svc, _ := NewOrganizationProfileService(store)
	_, err := svc.GetProfile(context.Background(), profileServiceOrg, profileServiceActor)
	if !errors.Is(err, models.ErrOrganizationProfileUnavailable) || strings.Contains(err.Error(), "secret") {
		t.Fatalf("store error leaked: %v", err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	store = &organizationProfileStoreStub{profile: organizationProfileServiceFixture()}
	svc, _ = NewOrganizationProfileService(store)
	_, err = svc.GetProfile(cancelled, profileServiceOrg, profileServiceActor)
	if !errors.Is(err, models.ErrOrganizationProfileUnavailable) || store.getCalls != 0 {
		t.Fatal("cancelled read reached store")
	}
}

func TestOrganizationProfileTimezoneDataAndFiscalDates(t *testing.T) {
	for _, zone := range []string{"UTC", "Africa/Lagos", "Europe/London", "America/New_York", "Etc/GMT+3"} {
		if !validProfileTimezone(zone) {
			t.Fatalf("valid zone rejected: %s", zone)
		}
	}
	for _, date := range [][2]int{{1, 1}, {2, 28}, {4, 30}, {12, 31}} {
		if !validFiscalStart(date[0], date[1]) {
			t.Fatal("valid fiscal date rejected")
		}
	}
	for _, date := range [][2]int{{0, 1}, {13, 1}, {1, 0}, {1, 32}, {2, 29}, {4, 31}} {
		if validFiscalStart(date[0], date[1]) {
			t.Fatal("invalid fiscal date accepted")
		}
	}
}

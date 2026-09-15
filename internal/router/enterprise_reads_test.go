package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/handler"
	"github.com/complianceforge/platform/internal/models"
)

type routerOrganizationProfileService struct{ calls int }

func (s routerOrganizationProfileService) GetProfile(_ context.Context, organization, _ string) (*models.OrganizationProfile, error) {
	return &models.OrganizationProfile{
		ID: organization, Name: "Profile tenant", Slug: "profile-tenant", LegalName: "Profile Tenant Legal",
		Industry: "Services", CountryCode: "NG", Timezone: "Africa/Lagos", DefaultLanguage: "en",
		SupportedLanguages: []string{"en"}, Status: "active", Tier: "enterprise", FiscalYearStartMonth: 1,
		FiscalYearStartDay: 1, Contacts: []models.OrganizationContact{}, Version: 1, UpdatedAt: time.Now().UTC(),
	}, nil
}

func (s routerOrganizationProfileService) UpdateProfile(ctx context.Context, organization, actor, _ string, input models.OrganizationProfileUpdateInput) (*models.OrganizationProfile, error) {
	profile, err := s.GetProfile(ctx, organization, actor)
	if input.Name != nil {
		profile.Name = *input.Name
	}
	profile.Version = input.ExpectedVersion + 1
	return profile, err
}

type routerDataQualityService struct{}

func (routerDataQualityService) GetSnapshot(context.Context, string, string) (*models.DataQualitySnapshot, error) {
	snapshot := &models.DataQualitySnapshot{
		RulesetVersion: models.DataQualityRulesetVersion, Scope: models.DataQualityScope,
		SchemaVersion: database.SupportedSchemaVersion, AsOf: time.Now().UTC(),
		Status: models.DataQualityHealthy, Checks: []models.DataQualityCheck{},
	}
	for _, definition := range models.DataQualityCheckDefinitions() {
		snapshot.Checks = append(snapshot.Checks, models.DataQualityCheck{DataQualityCheckDefinition: definition, Status: models.DataQualityHealthy})
	}
	return snapshot, nil
}

type routerCalendarReadService struct{}

func (routerCalendarReadService) ListEvents(context.Context, string, string, models.CalendarEventQuery, models.CalendarAccessContext) (*models.CalendarEventPage, error) {
	return &models.CalendarEventPage{
		Data: []models.CalendarEventView{}, Pagination: models.PaginationResponse{Page: 1, PageSize: 25},
		Meta: models.CalendarViewMeta{StartDate: "2026-09-01", EndDate: "2026-09-30", AsOf: time.Now().UTC(),
			DateTimezone: "UTC", SourceTable: "calendar_events", CandidateLimit: models.CalendarMaximumCandidates},
	}, nil
}

func (s routerCalendarReadService) Upcoming(ctx context.Context, org, actor string, query models.CalendarEventQuery, access models.CalendarAccessContext) (*models.CalendarEventPage, error) {
	return s.ListEvents(ctx, org, actor, query, access)
}

func (s routerCalendarReadService) Overdue(ctx context.Context, org, actor string, query models.CalendarEventQuery, access models.CalendarAccessContext) (*models.CalendarEventPage, error) {
	return s.ListEvents(ctx, org, actor, query, access)
}

func (s routerCalendarReadService) Summary(context.Context, string, string, models.CalendarEventQuery, models.CalendarAccessContext) (*models.CalendarSummaryResponse, error) {
	return &models.CalendarSummaryResponse{Data: models.CalendarSummaryView{
		ByEventType: map[string]int{}, ByCategory: map[string]int{}, ByPriority: map[string]int{}, ByStatus: map[string]int{}, Days: []models.CalendarDayCount{},
	}, Meta: models.CalendarViewMeta{StartDate: "2026-09-01", EndDate: "2026-09-30", AsOf: time.Now().UTC(),
		DateTimezone: "UTC", SourceTable: "calendar_events", CandidateLimit: models.CalendarMaximumCandidates}}, nil
}

func TestEnterpriseReadAndProfileRoutesAuthenticateAndAuthorize(t *testing.T) {
	for _, route := range []struct{ method, path, body, resource, action string }{
		{http.MethodGet, "/api/v1/settings/organization", "", "settings", "read"},
		{http.MethodPut, "/api/v1/settings/organization", `{"expected_version":1,"reason":"Approved review","name":"Reviewed profile"}`, "settings", "configure"},
		{http.MethodGet, "/api/v1/settings/data-quality", "", "settings", "read"},
		{http.MethodGet, "/api/v1/calendar/events", "", "audits", "read"},
		{http.MethodGet, "/api/v1/calendar/upcoming", "", "audits", "read"},
		{http.MethodGet, "/api/v1/calendar/overdue", "", "audits", "read"},
		{http.MethodGet, "/api/v1/calendar/summary", "", "audits", "read"},
	} {
		permission, ok := permissionForProtectedRequest(route.method, route.path)
		if !ok || permission.Resource != route.resource || permission.Action != route.action {
			t.Fatalf("unreviewed permission for %s %s: %+v", route.method, route.path, permission)
		}
		for _, test := range []struct {
			name         string
			token, allow bool
			expectedCode int
		}{
			{"missing identity", false, true, http.StatusUnauthorized},
			{"authenticated denied", true, false, http.StatusForbidden},
			{"authenticated allowed", true, true, http.StatusOK},
		} {
			t.Run(route.method+route.path+test.name, func(t *testing.T) {
				dependencies := testRouterDependencies()
				dependencies.Authorizer = &routerAuthorizer{allowed: test.allow}
				router, err := NewRouterWithDependencies(testRouterConfig(), dependencies)
				if err != nil {
					t.Fatal(err)
				}
				request := httptest.NewRequest(route.method, route.path, strings.NewReader(route.body))
				request.Header.Set("Content-Type", "application/json")
				request.Header.Set("X-API-Key", "cf_live_test")
				if test.token {
					request.Header.Set("Authorization", "Bearer test-token")
				}
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				if response.Code != test.expectedCode {
					t.Fatalf("status=%d expected=%d body=%s", response.Code, test.expectedCode, response.Body.String())
				}
			})
		}
	}
}

func TestEnterpriseReadAndProfileDependenciesRequiredIncludingTypedNil(t *testing.T) {
	var nilProfile *routerOrganizationProfileService
	var nilQuality *routerDataQualityService
	var nilCalendar *routerCalendarReadService
	for _, test := range []struct {
		name, required string
		unset          func(*RouterDependencies)
	}{
		{"profile absent", "organization profile handler is required", func(d *RouterDependencies) { d.OrganizationProfile = nil }},
		{"profile nil", "organization profile handler is required", func(d *RouterDependencies) { d.OrganizationProfile = handler.NewOrganizationProfileHandler(nil) }},
		{"profile typed nil", "organization profile handler is required", func(d *RouterDependencies) { d.OrganizationProfile = handler.NewOrganizationProfileHandler(nilProfile) }},
		{"quality absent", "data quality handler is required", func(d *RouterDependencies) { d.DataQuality = nil }},
		{"quality nil", "data quality handler is required", func(d *RouterDependencies) { d.DataQuality = handler.NewDataQualityHandler(nil) }},
		{"quality typed nil", "data quality handler is required", func(d *RouterDependencies) { d.DataQuality = handler.NewDataQualityHandler(nilQuality) }},
		{"calendar absent", "calendar read handler is required", func(d *RouterDependencies) { d.CalendarRead = nil }},
		{"calendar nil", "calendar read handler is required", func(d *RouterDependencies) { d.CalendarRead = handler.NewCalendarReadHandler(nil) }},
		{"calendar typed nil", "calendar read handler is required", func(d *RouterDependencies) { d.CalendarRead = handler.NewCalendarReadHandler(nilCalendar) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			dependencies := testRouterDependencies()
			test.unset(&dependencies)
			if err := dependencies.Validate(); err == nil || !strings.Contains(err.Error(), test.required) {
				t.Fatalf("unsafe startup dependency accepted: %v", err)
			}
		})
	}
}

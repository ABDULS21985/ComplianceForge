package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/complianceforge/platform/internal/authz"
	"github.com/complianceforge/platform/internal/middleware"
	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/service"
)

const profileHandlerOrg = "20000000-aaaa-4000-8000-000000000001"
const profileHandlerActor = "30000000-aaaa-4000-8000-000000000001"
const profileHandlerRequestID = "40000000-aaaa-4000-8000-000000000001"

type organizationProfileHandlerStub struct {
	profile             *models.OrganizationProfile
	err                 error
	calls               int
	org, actor, request string
	input               models.OrganizationProfileUpdateInput
}

func (s *organizationProfileHandlerStub) GetProfile(_ context.Context, org, actor string) (*models.OrganizationProfile, error) {
	s.calls++
	s.org, s.actor = org, actor
	return s.profile, s.err
}
func (s *organizationProfileHandlerStub) UpdateProfile(_ context.Context, org, actor, request string, input models.OrganizationProfileUpdateInput) (*models.OrganizationProfile, error) {
	s.calls++
	s.org, s.actor, s.request, s.input = org, actor, request, input
	return s.profile, s.err
}

func profileHandlerFixture() *models.OrganizationProfile {
	return &models.OrganizationProfile{ID: profileHandlerOrg, Name: "Tenant", Timezone: "Africa/Lagos", Version: 2, Contacts: []models.OrganizationContact{}, SupportedLanguages: []string{"en"}, FiscalYearStartMonth: 1, FiscalYearStartDay: 1, UpdatedAt: time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)}
}

func profileHandlerRequest(method, body string) *http.Request {
	r := httptest.NewRequest(method, "/api/v1/settings/organization", strings.NewReader(body))
	ctx := context.WithValue(r.Context(), middleware.ContextKeyOrgID, profileHandlerOrg)
	ctx = context.WithValue(ctx, middleware.ContextKeyUserID, profileHandlerActor)
	ctx = context.WithValue(ctx, middleware.ContextKeyRequestID, profileHandlerRequestID)
	ctx = middleware.ContextWithAuthorizationDecision(ctx, authz.Decision{Allowed: true})
	r = r.WithContext(ctx)
	r.Header.Set("Content-Type", "application/json")
	return r
}

func TestOrganizationProfileHandlerUsesTrustedContextAndSafeProjection(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		t.Run(method, func(t *testing.T) {
			stub := &organizationProfileHandlerStub{profile: profileHandlerFixture()}
			handler := NewOrganizationProfileHandler(stub)
			r := profileHandlerRequest(method, `{"expected_version":1,"reason":"Reviewed","name":"Revised"}`)
			response := httptest.NewRecorder()
			if method == http.MethodGet {
				handler.GetProfile(response, r)
			} else {
				handler.UpdateProfile(response, r)
			}
			if response.Code != http.StatusOK || stub.calls != 1 || stub.org != profileHandlerOrg || stub.actor != profileHandlerActor || response.Header().Get("Cache-Control") != "private, no-store" {
				t.Fatalf("unsafe response: %d %s", response.Code, response.Body.String())
			}
			if method == http.MethodPut && (stub.request != profileHandlerRequestID || stub.input.Name == nil || *stub.input.Name != "Revised" || stub.input.ExpectedVersion != 1) {
				t.Fatal("mutation did not use trusted context")
			}
			for _, forbidden := range []string{`"settings"`, `"branding"`, `"metadata"`, `"parent_organization_id"`} {
				if strings.Contains(response.Body.String(), forbidden) {
					t.Fatalf("unreviewed projection: %s", forbidden)
				}
			}
		})
	}
}

func TestOrganizationProfileHandlerRejectsAmbiguousAndUnreviewedBodiesBeforeService(t *testing.T) {
	for _, body := range []string{
		"", `[]`, `null`, `true`, `{}`, `{"expected_version":1} {}`,
		`{"expected_version":1,"expected_version":2,"reason":"Reviewed","name":"New"}`,
		`{"expected_version":1,"reason":"Reviewed","name":null}`,
		`{"expected_version":1,"reason":"Reviewed","contacts":null}`,
		`{"expected_version":1,"reason":"Reviewed","contacts":[{"purpose":"privacy","purpose":"security","email":"a@example.com"}]}`,
		`{"expected_version":1,"reason":"Reviewed","contacts":[{"purpose":"security","email":"a@example.com","host":"secret"}]}`,
		`{"expected_version":1,"reason":"Reviewed","status":"active"}`,
		`{"expected_version":1,"reason":"Reviewed","tier":"enterprise"}`,
		`{"expected_version":1,"reason":"Reviewed","settings":{"key":"secret"}}`,
		`{"expected_version":1,"reason":"Reviewed","organization_id":"other"}`,
		`{"expected_version":1.5,"reason":"Reviewed","name":"New"}`,
		`{"expected_version":9223372036854775808,"reason":"Reviewed","name":"New"}`,
		`{"expected_version":1,"reason":"Reviewed","name":[[[[[[[[[[[[[[[[[[[["deep"]]]]]]]]]]]]]]]]]]]]}`,
	} {
		t.Run(body, func(t *testing.T) {
			stub := &organizationProfileHandlerStub{profile: profileHandlerFixture()}
			response := httptest.NewRecorder()
			NewOrganizationProfileHandler(stub).UpdateProfile(response, profileHandlerRequest(http.MethodPut, body))
			// Presence/value validation lives in the service. Empty objects are
			// syntactically valid; this stub intentionally doesn't emulate it.
			if body == `{}` {
				if stub.calls != 1 {
					t.Fatal("valid object did not reach service validation")
				}
				return
			}
			if response.Code != http.StatusBadRequest || stub.calls != 0 || strings.Contains(response.Body.String(), "secret") {
				t.Fatalf("ambiguous body reached service: %d calls%d %s", response.Code, stub.calls, response.Body.String())
			}
		})
	}
}

func TestOrganizationProfileHandlerEnforcesBodyLimitAndMediaType(t *testing.T) {
	for _, test := range []struct {
		name, body, media string
		status            int
	}{
		{"oversized", strings.Repeat(" ", maximumOrganizationProfileBody+1), "application/json", http.StatusRequestEntityTooLarge},
		{"missing media", `{}`, "", http.StatusUnsupportedMediaType},
		{"wrong media", `{}`, "text/plain", http.StatusUnsupportedMediaType},
	} {
		t.Run(test.name, func(t *testing.T) {
			stub := &organizationProfileHandlerStub{}
			r := profileHandlerRequest(http.MethodPut, test.body)
			r.Header.Set("Content-Type", test.media)
			response := httptest.NewRecorder()
			NewOrganizationProfileHandler(stub).UpdateProfile(response, r)
			if response.Code != test.status || stub.calls != 0 {
				t.Fatalf("bad input reached service: %d", response.Code)
			}
		})
	}
}

func TestOrganizationProfileHandlerPreflightsAuthorizationBeforeMutation(t *testing.T) {
	for _, name := range []string{"no identity", "no decision", "denied", "unknown obligation", "malformed field", "hidden", "masked"} {
		t.Run(name, func(t *testing.T) {
			stub := &organizationProfileHandlerStub{profile: profileHandlerFixture()}
			r := profileHandlerRequest(http.MethodPut, `{"expected_version":1,"reason":"Reviewed","name":"New"}`)
			ctx := r.Context()
			switch name {
			case "no identity":
				ctx = context.Background()
			case "no decision":
				ctx = context.WithValue(context.Background(), middleware.ContextKeyOrgID, profileHandlerOrg)
				ctx = context.WithValue(ctx, middleware.ContextKeyUserID, profileHandlerActor)
			case "denied":
				ctx = middleware.ContextWithAuthorizationDecision(ctx, authz.Decision{})
			case "unknown obligation":
				ctx = middleware.ContextWithAuthorizationDecision(ctx, authz.Decision{Allowed: true, Obligations: []authz.Obligation{{Kind: "watermark"}}})
			case "malformed field":
				ctx = middleware.ContextWithAuthorizationDecision(ctx, authz.Decision{Allowed: true, Obligations: []authz.Obligation{{Kind: service.AccessFieldVisibilityObligation}}})
			case "hidden", "masked":
				visibility, strategy := models.AccessFieldHidden, models.AccessMaskStrategy("")
				if name == "masked" {
					visibility, strategy = models.AccessFieldMasked, models.AccessMaskRedact
				}
				ctx = middleware.ContextWithAuthorizationDecision(ctx, authz.Decision{Allowed: true, Obligations: service.AccessFieldObligations([]models.AccessFieldPermission{classifiedRule("settings", "contacts", visibility, strategy)})})
			}
			response := httptest.NewRecorder()
			NewOrganizationProfileHandler(stub).UpdateProfile(response, r.WithContext(ctx))
			if response.Code < 400 || stub.calls != 0 {
				t.Fatalf("unauthorized/restricted mutation reached service: %d", response.Code)
			}
		})
	}
}

func TestOrganizationProfileHandlerMasksReadsAndRejectsWrongTenantResults(t *testing.T) {
	profile := profileHandlerFixture()
	profile.Contacts = []models.OrganizationContact{{Purpose: "security", Email: "private@example.com"}}
	stub := &organizationProfileHandlerStub{profile: profile}
	r := profileHandlerRequest(http.MethodGet, "")
	r = r.WithContext(middleware.ContextWithAuthorizationDecision(r.Context(), authz.Decision{Allowed: true, Obligations: service.AccessFieldObligations([]models.AccessFieldPermission{classifiedRule("settings", "contacts", models.AccessFieldHidden, "")})}))
	response := httptest.NewRecorder()
	NewOrganizationProfileHandler(stub).GetProfile(response, r)
	if response.Code != 200 || strings.Contains(response.Body.String(), "private@example.com") ||
		!strings.Contains(response.Body.String(), `"editable":false`) || profile.Contacts[0].Email != "private@example.com" {
		t.Fatal("read masking failed or mutated shared data")
	}
	profile.ID = "50000000-aaaa-4000-8000-000000000001"
	response = httptest.NewRecorder()
	NewOrganizationProfileHandler(stub).GetProfile(response, profileHandlerRequest(http.MethodGet, ""))
	if response.Code != 503 || strings.Contains(response.Body.String(), profile.ID) {
		t.Fatal("wrong-tenant store result serialized")
	}
}

func TestOrganizationProfileHandlerMapsSafeErrorsAndTypedNil(t *testing.T) {
	var typedNil *organizationProfileHandlerStub
	if NewOrganizationProfileHandler(typedNil).Ready() {
		t.Fatal("typed nil handler dependency accepted")
	}
	for _, test := range []struct {
		err    error
		status int
		code   string
	}{
		{models.ErrOrganizationProfileScope, 403, "organization_profile_scope_denied"},
		{models.ErrOrganizationProfileNotFound, 404, "organization_profile_not_found"},
		{models.ErrOrganizationProfileConflict, 409, "organization_profile_conflict"},
		{&models.OrganizationProfileValidationError{Field: "name", Code: "secret-provider-detail"}, 422, "organization_profile_validation_failed"},
		{errors.New("password=secret; host=internal"), 503, "organization_profile_unavailable"},
	} {
		response := httptest.NewRecorder()
		NewOrganizationProfileHandler(&organizationProfileHandlerStub{err: test.err}).GetProfile(response, profileHandlerRequest(http.MethodGet, ""))
		if response.Code != test.status || !strings.Contains(response.Body.String(), test.code) || strings.Contains(response.Body.String(), "secret") || strings.Contains(response.Body.String(), "internal") {
			t.Fatalf("unsafe failure: %d %s", response.Code, response.Body.String())
		}
	}
}

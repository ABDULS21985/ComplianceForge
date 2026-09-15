package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/complianceforge/platform/internal/authz"
	"github.com/complianceforge/platform/internal/middleware"
	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/service"
)

const (
	classifiedOrgID  = "94000000-0000-0000-0000-000000000001"
	classifiedUserID = "94000000-0000-0000-0000-000000000002"
	classifiedItemID = "94000000-0000-0000-0000-000000000003"
)

type classifiedAuthorizer struct{ obligations []authz.Obligation }

func handlerAllowedContext(ctx context.Context) context.Context {
	return middleware.ContextWithAuthorizationDecision(ctx, authz.Decision{
		Allowed: true, Reason: "focused handler test", ReasonCode: "focused_handler_test",
		Obligations: []authz.Obligation{},
	})
}

func (a classifiedAuthorizer) Authorize(context.Context, authz.Request) (authz.Decision, error) {
	return authz.Decision{
		Allowed: true, Reason: "test allow", ReasonCode: "test_allow",
		Obligations: a.obligations,
	}, nil
}

type classifiedDirectoryService struct {
	UserAdministrationService
	item *models.DirectoryUser
}

func (s classifiedDirectoryService) GetUser(context.Context, string, string) (*models.DirectoryUser, error) {
	return s.item, nil
}

type classifiedVendorService struct {
	VendorService
	item *models.Vendor
}

func (s classifiedVendorService) GetByID(context.Context, string, string) (*models.Vendor, error) {
	return s.item, nil
}

type classifiedRiskService struct {
	RiskService
	item *models.Risk
}

func (s classifiedRiskService) GetByID(context.Context, string, string) (*models.Risk, error) {
	return s.item, nil
}

type classifiedIncidentService struct {
	IncidentService
	item *models.Incident
}

func (s classifiedIncidentService) GetByID(context.Context, string, string) (*models.Incident, error) {
	return s.item, nil
}

type classifiedOrganizationService struct {
	OrganizationService
	item *models.Organization
}

func (s classifiedOrganizationService) GetByID(context.Context, string) (*models.Organization, error) {
	return s.item, nil
}

type classifiedControlService struct {
	ControlService
	item     *models.Control
	evidence []models.ControlEvidence
}

func (s classifiedControlService) GetControl(context.Context, string, string) (*models.Control, error) {
	return s.item, nil
}

func (s classifiedControlService) ListControlEvidence(context.Context, string, string, models.PaginationRequest) ([]models.ControlEvidence, int, error) {
	return s.evidence, len(s.evidence), nil
}

func TestClassifiedDomainHandlersNeverSerializeHiddenOrUnmaskedValues(t *testing.T) {
	description := "Unreleased acquisition impact"
	directory := &models.DirectoryUser{
		TenantModel: models.TenantModel{BaseModel: models.BaseModel{ID: classifiedItemID}, OrganizationID: classifiedOrgID},
		Email:       "ada.private@example.test", Phone: "+2348012345678", FirstName: "Ada", LastName: "Private",
		Manager: &models.DirectoryUserReference{ID: classifiedUserID, Email: "manager.private@example.test"},
	}
	vendor := &models.Vendor{
		TenantModel:  models.TenantModel{BaseModel: models.BaseModel{ID: classifiedItemID}, OrganizationID: classifiedOrgID},
		ContactEmail: "primary.vendor@example.test", Contacts: []models.VendorContact{{Email: "nested.vendor@example.test"}},
	}
	risk := &models.Risk{
		TenantModel: models.TenantModel{BaseModel: models.BaseModel{ID: classifiedItemID}, OrganizationID: classifiedOrgID},
		Description: &description, Metadata: []byte(`{"credentials":{"token":"risk-token-secret"},"owner":"risk"}`),
	}
	incident := &models.Incident{
		TenantModel: models.TenantModel{BaseModel: models.BaseModel{ID: classifiedItemID}, OrganizationID: classifiedOrgID},
		Description: "Names and medical circumstances", DataCategories: []string{"health", "identity"},
	}
	organization := &models.Organization{
		BaseModel: models.BaseModel{ID: classifiedOrgID}, TaxID: "NG-123456789",
		HeadquartersAddress: map[string]any{"contacts": []any{map[string]any{"email": "hq.private@example.test", "type": "legal"}}},
	}
	control := &models.Control{
		BaseModel:   models.BaseModel{ID: classifiedItemID},
		Description: "Restricted control implementation narrative",
		Metadata:    []byte(`{"integration":{"credential":"control-secret"},"owner":"security"}`),
	}

	tests := []struct {
		name      string
		resource  string
		handler   http.Handler
		rules     []models.AccessFieldPermission
		secrets   []string
		expected  []string
		unchanged func() bool
	}{
		{
			name: "directory user and nested manager", resource: "users",
			handler: http.HandlerFunc(NewUserAdministrationHandler(classifiedDirectoryService{item: directory}).GetUser),
			rules: []models.AccessFieldPermission{
				classifiedRule("users", "email", models.AccessFieldHidden, ""),
				classifiedRule("users", "phone", models.AccessFieldMasked, models.AccessMaskLast4),
			},
			secrets:  []string{"ada.private@example.test", "manager.private@example.test", "+2348012345678"},
			expected: []string{"*******5678"},
			unchanged: func() bool {
				return directory.Email == "ada.private@example.test" && directory.Manager.Email == "manager.private@example.test"
			},
		},
		{
			name: "vendor and contact array", resource: "vendors",
			handler: http.HandlerFunc(NewVendorHandler(classifiedVendorService{item: vendor}).GetByID),
			rules: []models.AccessFieldPermission{
				classifiedRule("vendors", "contact_email", models.AccessFieldHidden, ""),
				classifiedRule("vendors", "contacts.email", models.AccessFieldMasked, models.AccessMaskEmail),
			},
			secrets:  []string{"primary.vendor@example.test", "nested.vendor@example.test"},
			expected: []string{"n***@example.test"},
			unchanged: func() bool {
				return vendor.ContactEmail == "primary.vendor@example.test" && vendor.Contacts[0].Email == "nested.vendor@example.test"
			},
		},
		{
			name: "risk and nested metadata", resource: "risks",
			handler: http.HandlerFunc(NewRiskHandler(classifiedRiskService{item: risk}).GetByID),
			rules: []models.AccessFieldPermission{
				classifiedRule("risks", "description", models.AccessFieldHidden, ""),
				classifiedRule("risks", "metadata.credentials.token", models.AccessFieldHidden, ""),
			},
			secrets:  []string{description, "risk-token-secret"},
			expected: []string{`"owner":"risk"`},
			unchanged: func() bool {
				return *risk.Description == description && strings.Contains(string(risk.Metadata), "risk-token-secret")
			},
		},
		{
			name: "incident and string array", resource: "incidents",
			handler: http.HandlerFunc(NewIncidentHandler(classifiedIncidentService{item: incident}).GetByID),
			rules: []models.AccessFieldPermission{
				classifiedRule("incidents", "description", models.AccessFieldHidden, ""),
				classifiedRule("incidents", "data_categories", models.AccessFieldMasked, models.AccessMaskRedact),
			},
			secrets:  []string{"Names and medical circumstances", "health", "identity"},
			expected: []string{`"data_categories":["***","***"]`},
			unchanged: func() bool {
				return incident.Description == "Names and medical circumstances" && incident.DataCategories[0] == "health"
			},
		},
		{
			name: "control and nested metadata", resource: "controls",
			handler: http.HandlerFunc(NewControlHandler(classifiedControlService{item: control}).GetByID),
			rules: []models.AccessFieldPermission{
				classifiedRule("controls", "description", models.AccessFieldHidden, ""),
				classifiedRule("controls", "metadata.integration.credential", models.AccessFieldHidden, ""),
			},
			secrets:  []string{"Restricted control implementation narrative", "control-secret"},
			expected: []string{`"owner":"security"`},
			unchanged: func() bool {
				return control.Description == "Restricted control implementation narrative" && strings.Contains(string(control.Metadata), "control-secret")
			},
		},
		{
			name: "organization nested map array", resource: "organizations",
			handler: http.HandlerFunc(NewOrganizationHandler(classifiedOrganizationService{item: organization}).GetByID),
			rules: []models.AccessFieldPermission{
				classifiedRule("organizations", "tax_id", models.AccessFieldMasked, models.AccessMaskLast4),
				classifiedRule("organizations", "headquarters_address.contacts.email", models.AccessFieldMasked, models.AccessMaskEmail),
			},
			secrets:  []string{"NG-123456789", "hq.private@example.test"},
			expected: []string{"********6789", "h***@example.test"},
			unchanged: func() bool {
				return organization.TaxID == "NG-123456789" && organization.HeadquartersAddress["contacts"].([]any)[0].(map[string]any)["email"] == "hq.private@example.test"
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := serveClassifiedHandler(test.resource, test.rules, test.handler)
			if response.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			for _, secret := range test.secrets {
				if strings.Contains(response.Body.String(), secret) {
					t.Fatalf("classified value %q leaked: %s", secret, response.Body.String())
				}
			}
			for _, expected := range test.expected {
				if !strings.Contains(response.Body.String(), expected) {
					t.Fatalf("expected %q in %s", expected, response.Body.String())
				}
			}
			if !test.unchanged() {
				t.Fatal("handler mutated the shared model returned by its service")
			}
		})
	}
}

func TestControlEvidenceListAppliesControlFieldObligationsWithoutMutatingRecord(t *testing.T) {
	objectKey := "tenant/quarantine/must-never-serialize"
	fileName := "executive-payroll.pdf"
	evidence := []models.ControlEvidence{{
		BaseModel: models.BaseModel{ID: classifiedItemID}, OrganizationID: classifiedOrgID,
		Title: "Payroll access review", ObjectKey: &objectKey, FileName: &fileName,
		Metadata: []byte(`{"subject":{"account":"sensitive-account"},"source":"manual"}`),
	}}
	handler := NewControlHandler(classifiedControlService{evidence: evidence})
	response := serveClassifiedHandler("controls", []models.AccessFieldPermission{
		classifiedRule("controls", "data.file_name", models.AccessFieldMasked, models.AccessMaskLast4),
		classifiedRule("controls", "data.metadata.subject.account", models.AccessFieldHidden, ""),
	}, http.HandlerFunc(handler.ListEvidence))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	for _, secret := range []string{fileName, "sensitive-account", objectKey} {
		if strings.Contains(response.Body.String(), secret) {
			t.Fatalf("evidence value %q leaked: %s", secret, response.Body.String())
		}
	}
	if !strings.Contains(response.Body.String(), "*****************.pdf") {
		t.Fatalf("masked filename missing: %s", response.Body.String())
	}
	if *evidence[0].FileName != fileName || !strings.Contains(string(evidence[0].Metadata), "sensitive-account") {
		t.Fatal("evidence source model was mutated")
	}
}

func TestClassifiedHandlerFailsClosedOnCrossResourceObligation(t *testing.T) {
	vendor := &models.Vendor{
		TenantModel:  models.TenantModel{BaseModel: models.BaseModel{ID: classifiedItemID}, OrganizationID: classifiedOrgID},
		ContactEmail: "must-not-leak@example.test",
	}
	rule := classifiedRule("risks", "contact_email", models.AccessFieldHidden, "")
	response := serveClassifiedHandler(
		"vendors", []models.AccessFieldPermission{rule},
		http.HandlerFunc(NewVendorHandler(classifiedVendorService{item: vendor}).GetByID),
	)
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), `"error_code":"response_masking_unavailable"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), vendor.ContactEmail) {
		t.Fatalf("classified value leaked on fail-closed path: %s", response.Body.String())
	}
}

func TestClassifiedHandlerFailsClosedWithoutAuthorizationDecision(t *testing.T) {
	vendor := &models.Vendor{
		TenantModel:  models.TenantModel{BaseModel: models.BaseModel{ID: classifiedItemID}, OrganizationID: classifiedOrgID},
		ContactEmail: "missing-decision-must-not-leak@example.test",
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/vendors/"+classifiedItemID, nil)
	ctx := context.WithValue(request.Context(), middleware.ContextKeyOrgID, classifiedOrgID)
	route := chi.NewRouteContext()
	route.URLParams.Add("id", classifiedItemID)
	request = request.WithContext(context.WithValue(ctx, chi.RouteCtxKey, route))
	response := httptest.NewRecorder()
	NewVendorHandler(classifiedVendorService{item: vendor}).GetByID(response, request)
	if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), vendor.ContactEmail) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func serveClassifiedHandler(resource string, rules []models.AccessFieldPermission, next http.Handler) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/"+resource+"/"+classifiedItemID, nil)
	ctx := context.WithValue(request.Context(), middleware.ContextKeyOrgID, classifiedOrgID)
	ctx = context.WithValue(ctx, middleware.ContextKeyUserID, classifiedUserID)
	route := chi.NewRouteContext()
	route.URLParams.Add("id", classifiedItemID)
	request = request.WithContext(context.WithValue(ctx, chi.RouteCtxKey, route))
	response := httptest.NewRecorder()
	middleware.RequireAuthorization(
		classifiedAuthorizer{obligations: service.AccessFieldObligations(rules)}, resource, "read", nil,
	)(next).ServeHTTP(response, request)
	return response
}

func classifiedRule(
	resource, fieldPath string,
	visibility models.AccessFieldVisibility,
	strategy models.AccessMaskStrategy,
) models.AccessFieldPermission {
	return models.AccessFieldPermission{
		ID: "field-" + strings.ReplaceAll(fieldPath, ".", "-"), PolicyID: "policy-classified",
		PolicyPriority: 100, ResourceType: resource, FieldPath: fieldPath,
		Classification: models.AccessFieldRestricted, Visibility: visibility, MaskStrategy: strategy,
	}
}
